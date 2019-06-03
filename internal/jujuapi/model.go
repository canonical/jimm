// Copyright 2017 Canonical Ltd.

package jujuapi

import (
	"context"
	"reflect"

	"github.com/juju/juju/apiserver/common"
	jujuparams "github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/network"
	"github.com/juju/juju/rpc"
	"github.com/juju/juju/rpc/rpcreflect"
	"gopkg.in/errgo.v1"

	"github.com/CanonicalLtd/jimm/internal/jem"
	"github.com/CanonicalLtd/jimm/internal/mongodoc"
	"github.com/CanonicalLtd/jimm/internal/servermon"
	"github.com/CanonicalLtd/jimm/params"
)

// modelRoot is the root for endpoints served on model connections.
type modelRoot struct {
	authContext  context.Context
	jem          *jem.JEM
	uuid         string
	model        *mongodoc.Model
	controller   *mongodoc.Controller
	heartMonitor heartMonitor
	findMethod   func(rootName string, version int, methodName string) (rpcreflect.MethodCaller, error)
}

func newModelRoot(jem *jem.JEM, hm heartMonitor, uuid string) *modelRoot {
	r := &modelRoot{
		jem:          jem,
		uuid:         uuid,
		heartMonitor: hm,
	}
	r.findMethod = rpcreflect.ValueOf(reflect.ValueOf(r)).FindMethod
	return r
}

// Admin returns an implementation of the Admin facade (version 3).
func (r *modelRoot) Admin(id string) (modelAdmin, error) {
	if id != "" {
		// Safeguard id for possible future use.
		return modelAdmin{}, common.ErrBadId
	}
	return modelAdmin{r}, nil
}

// Pinger returns an implementation of the Pinger facade (version 1).
func (r *modelRoot) Pinger(id string) (pinger, error) {
	if id != "" {
		// Safeguard id for possible future use.
		return pinger{}, common.ErrBadId
	}
	return pinger{}, nil
}

// FindMethod implements rpcreflect.MethodFinder.
func (r *modelRoot) FindMethod(rootName string, version int, methodName string) (rpcreflect.MethodCaller, error) {
	// update the heart monitor for every request received.
	r.heartMonitor.Heartbeat()

	if rootName == "Admin" && version < 3 {
		return nil, &rpc.RequestError{
			Code:    jujuparams.CodeNotSupported,
			Message: "JIMM does not support login from old clients",
		}
	}

	return r.findMethod(rootName, 0, methodName)
}

// Kill implements rpcreflect.Root.Kill.
func (r *modelRoot) Kill() {}

// modelInfo retrieves the data about the model.
func (r *modelRoot) modelInfo(ctx context.Context) (*mongodoc.Model, *mongodoc.Controller, error) {
	if r.model == nil {
		var err error
		r.model, err = r.jem.DB.ModelFromUUID(ctx, r.uuid)
		if errgo.Cause(err) == params.ErrNotFound {
			return nil, nil, errgo.WithCausef(err, params.ErrModelNotFound, "%s", "")
		}
		if err != nil {
			return nil, nil, errgo.Mask(err)
		}
		r.controller, err = r.jem.DB.Controller(ctx, r.model.Controller)
		if err != nil {
			return nil, nil, errgo.Mask(err)
		}
	}
	return r.model, r.controller, nil
}

type modelAdmin struct {
	root *modelRoot
}

// Login implements the Login method on the Admin facade.
func (a modelAdmin) Login(ctx context.Context, req jujuparams.LoginRequest) (jujuparams.LoginResult, error) {
	_, _, err := a.root.modelInfo(ctx)
	if err != nil {
		return jujuparams.LoginResult{}, errgo.Mask(err, errgo.Is(params.ErrModelNotFound))
	}
	// If the model was found then we'll need to redirect to it.
	servermon.LoginRedirectCount.Inc()
	return jujuparams.LoginResult{}, &jujuparams.Error{
		Code:    jujuparams.CodeRedirect,
		Message: "redirection required",
	}
}

// RedirectInfo implements the RedirectInfo method on the Admin facade.
func (a modelAdmin) RedirectInfo(ctx context.Context) (jujuparams.RedirectInfoResult, error) {
	_, controller, err := a.root.modelInfo(ctx)
	if err != nil {
		return jujuparams.RedirectInfoResult{}, errgo.Mask(err, errgo.Is(params.ErrModelNotFound))
	}
	servers := make([][]jujuparams.HostPort, len(controller.HostPorts))
	for i, hps := range controller.HostPorts {
		servers[i] = make([]jujuparams.HostPort, len(hps))
		for j, hp := range hps {
			servers[i][j] = jujuparams.HostPort{
				Address: jujuparams.Address{
					Value: hp.Host,
					Scope: hp.Scope,
					Type:  string(network.DeriveAddressType(hp.Host)),
				},
				Port: hp.Port,
			}
		}
	}
	return jujuparams.RedirectInfoResult{
		Servers: servers,
		CACert:  controller.CACert,
	}, nil
}
