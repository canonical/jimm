// Copyright 2016 Canonical Ltd.

package jujuapi

import (
	"context"
	"sync"
	"time"

	jujuparams "github.com/juju/juju/apiserver/params"
	jujustatus "github.com/juju/juju/core/status"
	"github.com/juju/juju/environs/config"
	"github.com/juju/names/v4"
	"github.com/juju/rpcreflect"
	"github.com/juju/version"
	"github.com/rogpeppe/fastuuid"
	"go.uber.org/zap"
	"gopkg.in/errgo.v1"
	"gopkg.in/macaroon-bakery.v2/bakery/identchecker"

	"github.com/CanonicalLtd/jimm/internal/apiconn"
	"github.com/CanonicalLtd/jimm/internal/auth"
	"github.com/CanonicalLtd/jimm/internal/conv"
	"github.com/CanonicalLtd/jimm/internal/jem"
	"github.com/CanonicalLtd/jimm/internal/jemserver"
	"github.com/CanonicalLtd/jimm/internal/jujuapi/rpc"
	"github.com/CanonicalLtd/jimm/internal/mongodoc"
	"github.com/CanonicalLtd/jimm/internal/zapctx"
	"github.com/CanonicalLtd/jimm/params"
)

// controllerRoot is the root for endpoints served on controller connections.
type controllerRoot struct {
	rpc.Root

	params       jemserver.Params
	auth         *auth.Authenticator
	jem          *jem.JEM
	heartMonitor heartMonitor
	watchers     *watcherRegistry

	// mu protects the fields below it
	mu                    sync.Mutex
	identity              identchecker.ACLIdentity
	controllerUUIDMasking bool
	generator             *fastuuid.Generator
}

func newControllerRoot(jem *jem.JEM, a *auth.Authenticator, p jemserver.Params, hm heartMonitor) *controllerRoot {
	r := &controllerRoot{
		params:       p,
		auth:         a,
		jem:          jem,
		heartMonitor: hm,
		watchers: &watcherRegistry{
			watchers: make(map[string]*modelSummaryWatcher),
		},
		controllerUUIDMasking: true,
	}

	r.AddMethod("Admin", 1, "Login", rpc.Method(unsupportedLogin))
	r.AddMethod("Admin", 2, "Login", rpc.Method(unsupportedLogin))
	r.AddMethod("Admin", 3, "Login", rpc.Method(r.Login))
	r.AddMethod("Pinger", 1, "Ping", rpc.Method(ping))
	return r
}

// modelWithConnection gets the model with the given model tag, opens a
// connection to the model and runs the given function with the model and
// connection. The function will not have any error cause masked.
func (r *controllerRoot) modelWithConnection(ctx context.Context, modelTag string, access jujuparams.UserAccessPermission, f func(ctx context.Context, conn *apiconn.Conn, model *mongodoc.Model) error) error {
	mt, err := names.ParseModelTag(modelTag)
	if err != nil {
		return errgo.WithCausef(err, params.ErrBadRequest, "")
	}
	model := mongodoc.Model{UUID: mt.Id()}
	if err := r.jem.GetModel(ctx, r.identity, access, &model); err != nil {
		return errgo.Mask(err,
			errgo.Is(params.ErrNotFound),
			errgo.Is(params.ErrUnauthorized),
		)
	}
	conn, err := r.jem.OpenAPI(ctx, model.Controller)
	if err != nil {
		return errgo.Mask(err)
	}
	defer conn.Close()

	return errgo.Mask(f(ctx, conn, &model), errgo.Any)
}

// doModels calls the given function for each model that the
// authenticated user has access to. If f returns an error, the iteration
// will be stopped and the returned error will have the same cause.
func (r *controllerRoot) doModels(ctx context.Context, f func(context.Context, *mongodoc.Model) error) error {
	it := r.jem.DB.NewCanReadIter(ctx, r.jem.DB.Models().Find(nil).Sort("_id").Iter())
	defer it.Close(ctx)

	for {
		var model mongodoc.Model
		if !it.Next(ctx, &model) {
			break
		}
		if err := f(ctx, &model); err != nil {
			return errgo.Mask(err, errgo.Any)
		}
	}
	return errgo.Mask(it.Err(ctx))
}

// FindMethod implements rpcreflect.MethodFinder.
func (r *controllerRoot) FindMethod(rootName string, version int, methodName string) (rpcreflect.MethodCaller, error) {
	// update the heart monitor for every request received.
	r.heartMonitor.Heartbeat()
	return r.Root.FindMethod(rootName, version, methodName)
}

func userModelForModelDoc(m *mongodoc.Model) jujuparams.Model {
	return jujuparams.Model{
		Name:     string(m.Path.Name),
		UUID:     m.UUID,
		Type:     m.Type,
		OwnerTag: conv.ToUserTag(m.Path.User).String(),
	}
}

// newTime returns a pointer to t if it's non-zero,
// or nil otherwise.
func newTime(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}

func modelStatus(info *mongodoc.ModelInfo) jujuparams.EntityStatus {
	var status jujuparams.EntityStatus
	if info == nil {
		return status
	}
	status.Status = jujustatus.Status(info.Status.Status)
	status.Info = info.Status.Message
	status.Data = info.Status.Data
	if !info.Status.Since.IsZero() {
		status.Since = &info.Status.Since
	}
	return status
}

func modelVersion(ctx context.Context, info *mongodoc.ModelInfo) *version.Number {
	if info == nil {
		return nil
	}
	versionString, _ := info.Config[config.AgentVersionKey].(string)
	if versionString == "" {
		return nil
	}
	v, err := version.Parse(versionString)
	if err != nil {
		zapctx.Warn(ctx, "cannot parse agent-version", zap.String("agent-version", versionString), zap.Error(err))
		return nil
	}
	return &v
}
