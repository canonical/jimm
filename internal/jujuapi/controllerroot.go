// Copyright 2016 Canonical Ltd.

package jujuapi

import (
	"context"
	"sort"
	"sync"
	"time"

	modelmanagerapi "github.com/juju/juju/api/modelmanager"
	jujuparams "github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/life"
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
	"github.com/CanonicalLtd/jimm/internal/zaputil"
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

// modelInfo retrieves the model information for the specified entity.
func (r *controllerRoot) modelInfo(ctx context.Context, arg jujuparams.Entity, localOnly bool) (*jujuparams.ModelInfo, error) {
	mt, err := names.ParseModelTag(arg.Tag)
	if err != nil {
		return nil, errgo.WithCausef(err, params.ErrBadRequest, "")
	}
	model := mongodoc.Model{UUID: mt.Id()}
	if err := r.jem.GetModel(ctx, r.identity, jujuparams.ModelReadAccess, &model); err != nil {
		return nil, errgo.Mask(err,
			errgo.Is(params.ErrUnauthorized),
			errgo.Is(params.ErrNotFound),
		)
	}
	ctx = zapctx.WithFields(ctx, zap.String("model-uuid", model.UUID))
	info, err := r.modelDocToModelInfo(ctx, &model)
	if err != nil {
		return nil, errgo.Mask(err)
	}
	if localOnly {
		return info, nil
	}
	// Query the model itself for user information.
	infoFromController, err := fetchModelInfo(ctx, r.jem, &model)
	if err != nil {
		code := jujuparams.ErrCode(err)
		if model.Life() == string(life.Dying) && code == jujuparams.CodeUnauthorized {
			zapctx.Info(ctx, "could not get ModelInfo for dying model, marking dead", zap.Error(err))
			// The model was dying and now cannot be accessed, assume it is now dead.
			if err := r.jem.DB.DeleteModelWithUUID(ctx, model.Controller, model.UUID); err != nil {
				// If this update fails then don't worry as the watcher
				// will detect the state change and update as appropriate.
				zapctx.Warn(ctx, "error deleting model", zap.Error(err))
			}
			// return the error with the an appropriate cause.
			return nil, errgo.WithCausef(err, params.ErrUnauthorized, "%s", "")
		}

		// We have most of the information we want already so return that.
		zapctx.Error(ctx, "failed to get ModelInfo from controller", zap.String("controller", model.Controller.String()), zaputil.Error(err))
		return info, nil
	}
	info.Users = filterUsers(ctx, r.identity, infoFromController.Users, isModelAdmin(ctx, r.identity, infoFromController))
	return info, nil
}

func (r *controllerRoot) modelDocToModelInfo(ctx context.Context, model *mongodoc.Model) (*jujuparams.ModelInfo, error) {
	machines, err := r.jem.DB.MachinesForModel(ctx, model.UUID)
	if err != nil {
		return nil, errgo.Mask(err)
	}
	providerType := model.ProviderType
	if providerType == "" {
		providerType, err = r.jem.DB.ProviderType(ctx, model.Cloud)
		if err != nil {
			return nil, errgo.Notef(err, "cannot get cloud %q", model.Cloud)
		}
	}

	userLevels := make(map[string]jujuparams.UserAccessPermission)
	for _, user := range model.ACL.Read {
		userLevels[user] = jujuparams.ModelReadAccess
	}
	for _, user := range model.ACL.Write {
		userLevels[user] = jujuparams.ModelWriteAccess
	}
	for _, user := range model.ACL.Admin {
		userLevels[user] = jujuparams.ModelAdminAccess
	}
	userLevels[string(model.Path.User)] = jujuparams.ModelAdminAccess

	var users []jujuparams.ModelUserInfo
	if auth.CheckIsAdmin(ctx, r.identity, model) == nil {
		usernames := make([]string, 0, len(userLevels))
		for user := range userLevels {
			usernames = append(usernames, user)
		}
		sort.Strings(usernames)
		for _, user := range usernames {
			ut := userTag(user)
			users = append(users, jujuparams.ModelUserInfo{
				UserName:    ut.Id(),
				DisplayName: ut.Name(),
				Access:      userLevels[user],
			})
		}
	} else {
		ut := userTag(r.identity.Id())
		users = append(users, jujuparams.ModelUserInfo{
			UserName:    ut.Id(),
			DisplayName: ut.Name(),
			Access:      userLevels[r.identity.Id()],
		})
	}
	info := &jujuparams.ModelInfo{
		Name:               string(model.Path.Name),
		UUID:               model.UUID,
		ControllerUUID:     r.params.ControllerUUID,
		ProviderType:       providerType,
		DefaultSeries:      model.DefaultSeries,
		CloudTag:           conv.ToCloudTag(model.Cloud).String(),
		CloudRegion:        model.CloudRegion,
		CloudCredentialTag: conv.ToCloudCredentialTag(model.Credential.ToParams()).String(),
		OwnerTag:           conv.ToUserTag(model.Path.User).String(),
		Life:               life.Value(model.Life()),
		Status:             modelStatus(model.Info),
		Users:              users,
		Machines:           jemMachinesToModelMachineInfo(machines),
		AgentVersion:       modelVersion(ctx, model.Info),
		Type:               model.Type,
	}
	if !r.controllerUUIDMasking {
		c, err := r.jem.DB.Controller(ctx, model.Controller)
		if err != nil {
			return nil, errgo.Notef(err, "failed to fetch controller: %v", model.Controller)
		}
		info.ControllerUUID = c.UUID
	}

	return info, nil
}

func jemMachinesToModelMachineInfo(machines []mongodoc.Machine) []jujuparams.ModelMachineInfo {
	infos := make([]jujuparams.ModelMachineInfo, 0, len(machines))
	for _, m := range machines {
		if m.Info.Life != "dead" {
			infos = append(infos, jemMachineToModelMachineInfo(m))
		}
	}
	return infos
}

func jemMachineToModelMachineInfo(m mongodoc.Machine) jujuparams.ModelMachineInfo {
	var hardware *jujuparams.MachineHardware
	if m.Info.HardwareCharacteristics != nil {
		hardware = &jujuparams.MachineHardware{
			Arch:             m.Info.HardwareCharacteristics.Arch,
			Mem:              m.Info.HardwareCharacteristics.Mem,
			RootDisk:         m.Info.HardwareCharacteristics.RootDisk,
			Cores:            m.Info.HardwareCharacteristics.CpuCores,
			CpuPower:         m.Info.HardwareCharacteristics.CpuPower,
			Tags:             m.Info.HardwareCharacteristics.Tags,
			AvailabilityZone: m.Info.HardwareCharacteristics.AvailabilityZone,
		}
	}
	return jujuparams.ModelMachineInfo{
		Id:         m.Info.Id,
		InstanceId: m.Info.InstanceId,
		Status:     string(m.Info.AgentStatus.Current),
		HasVote:    m.Info.HasVote,
		WantsVote:  m.Info.WantsVote,
		Hardware:   hardware,
	}
}

// isModelAdmin determines if the current user is an admin on the given model.
func isModelAdmin(ctx context.Context, id identchecker.ACLIdentity, info *jujuparams.ModelInfo) bool {
	var admin bool
	iterUsers(ctx, info.Users, func(u params.User, ui jujuparams.ModelUserInfo) {
		admin = admin || ui.Access == jujuparams.ModelAdminAccess && auth.CheckIsUser(ctx, id, u) == nil
	})
	return admin
}

// filterUsers returns a slice holding all of the given users that the
// current user should be able to see. Admin users can see everyone;
// other users can only see users and groups they're a member of. Users
// local to the controller are always removed.
func filterUsers(ctx context.Context, id identchecker.ACLIdentity, users []jujuparams.ModelUserInfo, admin bool) []jujuparams.ModelUserInfo {
	filtered := make([]jujuparams.ModelUserInfo, 0, len(users))
	iterUsers(ctx, users, func(u params.User, ui jujuparams.ModelUserInfo) {
		if admin || auth.CheckIsUser(ctx, id, u) == nil {
			filtered = append(filtered, ui)
		}
	})
	return filtered
}

// iterUsers iterates through all the non-local users in users and calls
// f with each in turn.
func iterUsers(ctx context.Context, users []jujuparams.ModelUserInfo, f func(params.User, jujuparams.ModelUserInfo)) {
	for _, u := range users {
		user, err := conv.FromUserID(u.UserName)
		if err != nil {
			if errgo.Cause(err) != conv.ErrLocalUser {
				zapctx.Info(ctx, "controller sent invalid username, skipping", zap.String("username", u.UserName))
			}
			continue
		}
		f(user, u)
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

func fetchModelInfo(ctx context.Context, jem *jem.JEM, model *mongodoc.Model) (*jujuparams.ModelInfo, error) {
	conn, err := jem.OpenAPI(ctx, model.Controller)
	if err != nil {
		return nil, errgo.Mask(err, errgo.Is(context.DeadlineExceeded))
	}
	defer conn.Close()
	client := modelmanagerapi.NewClient(conn)
	var infos []jujuparams.ModelInfoResult
	err = runWithContext(ctx, func() error {
		var err error
		infos, err = client.ModelInfo([]names.ModelTag{names.NewModelTag(model.UUID)})
		return err
	})
	if err != nil {
		return nil, errgo.Mask(err, errgo.Is(context.DeadlineExceeded))
	}
	if len(infos) != 1 {
		return nil, errgo.Newf("unexpected number of ModelInfo results")
	}
	if infos[0].Error != nil {
		return nil, infos[0].Error
	}
	return infos[0].Result, nil
}

// userTag creates a UserTag from the given username. The returned
// UserTag will always have a domain set. If username has no domain then
// @external will be used.
func userTag(username string) names.UserTag {
	tag := names.NewUserTag(username)
	if tag.Domain() == "" {
		tag = tag.WithDomain("external")
	}
	return tag
}

// runWithContext runs the given function and completes either when the
// function completes, or when the given context is canceled. If the
// function returns because the context was cancelled then the returned
// error will have the value of ctx.Err().
func runWithContext(ctx context.Context, f func() error) error {
	c := make(chan error)
	go func() {
		err := f()
		select {
		case c <- err:
		case <-ctx.Done():
			if err != nil {
				zapctx.Info(ctx, "error in canceled task", zaputil.Error(err))
			}
		}
	}()
	select {
	case err := <-c:
		return errgo.Mask(err, errgo.Any)
	case <-ctx.Done():
		return errgo.Mask(ctx.Err(), errgo.Any)
	}
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
