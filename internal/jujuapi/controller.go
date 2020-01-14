// Copyright 2016 Canonical Ltd.

package jujuapi

import (
	"context"
	"reflect"
	"sort"
	"sync"

	modelmanagerapi "github.com/juju/juju/api/modelmanager"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facades/client/bundle"
	jujuparams "github.com/juju/juju/apiserver/params"
	jujucloud "github.com/juju/juju/cloud"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/permission"
	jujustatus "github.com/juju/juju/core/status"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/rpc"
	"github.com/juju/rpcreflect"
	"github.com/juju/version"
	"go.uber.org/zap"
	"gopkg.in/errgo.v1"
	"gopkg.in/juju/names.v3"
	"gopkg.in/macaroon-bakery.v2/bakery"
	"gopkg.in/mgo.v2/bson"

	"github.com/CanonicalLtd/jimm/internal/auth"
	"github.com/CanonicalLtd/jimm/internal/ctxutil"
	"github.com/CanonicalLtd/jimm/internal/jem"
	"github.com/CanonicalLtd/jimm/internal/jemserver"
	"github.com/CanonicalLtd/jimm/internal/mongodoc"
	"github.com/CanonicalLtd/jimm/internal/servermon"
	"github.com/CanonicalLtd/jimm/internal/zapctx"
	"github.com/CanonicalLtd/jimm/internal/zaputil"
	"github.com/CanonicalLtd/jimm/params"
)

type facade struct {
	name    string
	version int
}

// unauthenticatedfacades contains the list of facade versions supported by
// this API before the user has authenticated.
var unauthenticatedFacades = map[facade]string{
	{"Admin", 3}:  "Admin",
	{"Pinger", 1}: "Pinger",
}

// facades contains the list of facade versions supported by
// this API. The value associated with each key holds the
// name of the top level method to call on controllerRoot
// to obtain the facade object.
var facades = map[facade]string{
	{"Bundle", 1}:       "Bundle",
	{"Cloud", 1}:        "CloudV1",
	{"Cloud", 2}:        "CloudV2",
	{"Cloud", 3}:        "CloudV3",
	{"Cloud", 4}:        "CloudV4",
	{"Cloud", 5}:        "CloudV5",
	{"Controller", 3}:   "Controller",
	{"JIMM", 1}:         "JIMM",
	{"ModelManager", 2}: "ModelManagerV2",
	{"ModelManager", 3}: "ModelManagerV3",
	{"ModelManager", 4}: "ModelManagerAPI",
	{"ModelManager", 5}: "ModelManagerAPI",
	{"Pinger", 1}:       "Pinger",
	{"UserManager", 1}:  "UserManager",
}

// controllerRoot is the root for endpoints served on controller connections.
type controllerRoot struct {
	params       jemserver.Params
	auth         *auth.Authenticator
	jem          *jem.JEM
	heartMonitor heartMonitor

	findMethod    func(rootName string, version int, methodName string) (rpcreflect.MethodCaller, error)
	schemataCache map[params.Cloud]map[jujucloud.AuthType]jujucloud.CredentialSchema

	// mu protects the fields below it
	mu          sync.Mutex
	authContext context.Context
	facades     map[facade]string
}

func newControllerRoot(jem *jem.JEM, a *auth.Authenticator, p jemserver.Params, hm heartMonitor) *controllerRoot {
	r := &controllerRoot{
		authContext:   context.Background(),
		params:        p,
		auth:          a,
		jem:           jem,
		heartMonitor:  hm,
		facades:       unauthenticatedFacades,
		schemataCache: make(map[params.Cloud]map[jujucloud.AuthType]jujucloud.CredentialSchema),
	}
	r.findMethod = rpcreflect.ValueOf(reflect.ValueOf(r)).FindMethod
	return r
}

// Admin returns an implementation of the Admin facade (version 3).
func (r *controllerRoot) Admin(id string) (admin, error) {
	if id != "" {
		// Safeguard id for possible future use.
		return admin{}, common.ErrBadId
	}
	return admin{r}, nil
}

// Bundle returns an implementation of the Bundle facade (version 1).
func (r *controllerRoot) Bundle(id string) (*bundle.APIv1, error) {
	if id != "" {
		// Safeguard id for possible future use.
		return nil, common.ErrBadId
	}
	// Use the juju implementation of the Bundle facade.
	api, err := bundle.NewBundleAPIv1(nil, authorizer{r.authContext}, names.NewModelTag(""))
	return api, errgo.Mask(err)
}

// Controller returns an implementation of the Controller facade (version 1).
func (r *controllerRoot) Controller(id string) (controller, error) {
	if id != "" {
		// Safeguard id for possible future use.
		return controller{}, common.ErrBadId
	}
	return controller{r}, nil
}

// JIMM returns an implementation of the JIMM-specific
// API facade.
func (r *controllerRoot) JIMM(id string) (jimm, error) {
	if id != "" {
		// Safeguard id for possible future use.
		return jimm{}, common.ErrBadId
	}
	return jimm{r}, nil
}

// Pinger returns an implementation of the Pinger facade (version 1).
func (r *controllerRoot) Pinger(id string) (pinger, error) {
	if id != "" {
		// Safeguard id for possible future use.
		return pinger{}, common.ErrBadId
	}
	return pinger{}, nil
}

// UserManager returns an implementation of the UserManager facade
// (version 1).
func (r *controllerRoot) UserManager(id string) (userManager, error) {
	if id != "" {
		// Safeguard id for possible future use.
		return userManager{}, common.ErrBadId
	}
	return userManager{r}, nil
}

// doModels calls the given function for each model that the
// authenticated user has access to. If f returns an error, the iteration
// will be stopped and the returned error will have the same cause.
func (r *controllerRoot) doModels(ctx context.Context, f func(context.Context, *mongodoc.Model) error) error {
	it := r.jem.DB.NewCanReadIter(ctx, r.jem.DB.Models().Find(nil).Sort("_id").Iter())
	defer it.Close()

	for {
		var model mongodoc.Model
		if !it.Next(&model) {
			break
		}
		if err := f(ctx, &model); err != nil {
			return errgo.Mask(err, errgo.Any)
		}
	}
	return errgo.Mask(it.Err())
}

// FindMethod implements rpcreflect.MethodFinder.
func (r *controllerRoot) FindMethod(rootName string, version int, methodName string) (rpcreflect.MethodCaller, error) {
	// update the heart monitor for every request received.
	r.heartMonitor.Heartbeat()

	if rootName == "Admin" && version < 3 {
		return nil, &rpc.RequestError{
			Code:    jujuparams.CodeNotSupported,
			Message: "JIMM does not support login from old clients",
		}
	}
	r.mu.Lock()
	rn := r.facades[facade{rootName, version}]
	r.mu.Unlock()
	if rn == "" {
		return nil, &rpcreflect.CallNotImplementedError{
			RootMethod: rootName,
			Version:    version,
		}
	}
	return r.findMethod(rn, 0, methodName)
}

// credentialSchema gets the schema for the credential identified by the
// given cloud and authType.
func (r *controllerRoot) credentialSchema(ctx context.Context, cloud params.Cloud, authType string) (jujucloud.CredentialSchema, error) {
	if cs, ok := r.schemataCache[cloud]; ok {
		return cs[jujucloud.AuthType(authType)], nil
	}
	providerType, err := r.jem.DB.ProviderType(ctx, cloud)
	if err != nil {
		return nil, errgo.Mask(err, errgo.Is(params.ErrNotFound))
	}
	provider, err := environs.Provider(providerType)
	if err != nil {
		return nil, errgo.Mask(err)
	}
	r.schemataCache[cloud] = provider.CredentialSchemas()
	return r.schemataCache[cloud][jujucloud.AuthType(authType)], nil
}

// Kill implements rpcreflect.Root.Kill.
func (r *controllerRoot) Kill() {}

// admin implements the Admin facade.
type admin struct {
	root *controllerRoot
}

// Login implements the Login method on the Admin facade.
func (a admin) Login(ctx context.Context, req jujuparams.LoginRequest) (jujuparams.LoginResult, error) {
	// JIMM only supports macaroon login, ignore all the other fields.
	authContext, m, err := a.root.auth.Authenticate(a.root.authContext, bakery.Version1, req.Macaroons)
	if err != nil {
		servermon.LoginFailCount.Inc()
		if m != nil {
			return jujuparams.LoginResult{
				DischargeRequired:       m.M(),
				DischargeRequiredReason: err.Error(),
			}, nil
		}
		return jujuparams.LoginResult{}, errgo.Mask(err)
	}
	a.root.mu.Lock()
	a.root.facades = facades
	a.root.authContext = authContext
	a.root.mu.Unlock()

	ctx = ctxutil.Join(ctx, a.root.authContext)
	servermon.LoginSuccessCount.Inc()
	username := auth.Username(ctx)
	srvVersion, err := a.root.jem.EarliestControllerVersion(ctx)
	if err != nil {
		return jujuparams.LoginResult{}, errgo.Mask(err)
	}
	return jujuparams.LoginResult{
		UserInfo: &jujuparams.AuthUserInfo{
			// TODO(mhilton) get a better display name from the identity manager.
			DisplayName: username,
			Identity:    userTag(username).String(),
		},
		ControllerTag: names.NewControllerTag(a.root.params.ControllerUUID).String(),
		Facades:       facadeVersions(a.root.facades),
		ServerVersion: srvVersion.String(),
	}, nil
}

// facadeVersions creates a list of facadeVersions as specified in
// facades.
func facadeVersions(facades map[facade]string) []jujuparams.FacadeVersions {
	names := make([]string, 0, len(facades))
	versions := make(map[string][]int, len(facades))
	for k := range facades {
		vs, ok := versions[k.name]
		if !ok {
			names = append(names, k.name)
		}
		versions[k.name] = append(vs, k.version)
	}
	sort.Strings(names)
	fvs := make([]jujuparams.FacadeVersions, len(names))
	for i, name := range names {
		vs := versions[name]
		sort.Ints(vs)
		fvs[i] = jujuparams.FacadeVersions{
			Name:     name,
			Versions: vs,
		}
	}
	return fvs
}

// controller implements the Controller facade.
type controller struct {
	root *controllerRoot
}

func (c controller) AllModels(ctx context.Context) (jujuparams.UserModelList, error) {
	ctx = ctxutil.Join(ctx, c.root.authContext)
	return c.root.allModels(ctx)
}

func (c controller) ModelStatus(ctx context.Context, args jujuparams.Entities) (jujuparams.ModelStatusResults, error) {
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	ctx = ctxutil.Join(ctx, c.root.authContext)
	results := make([]jujuparams.ModelStatus, len(args.Entities))
	// TODO (fabricematrat) get status for all of the models connected
	// to a single controller in one go.
	for i, arg := range args.Entities {
		mi, err := c.modelStatus(ctx, arg)
		if err != nil {
			return jujuparams.ModelStatusResults{}, errgo.Mask(err, errgo.Is(params.ErrNotFound))
		}
		results[i] = *mi
	}

	return jujuparams.ModelStatusResults{
		Results: results,
	}, nil
}

// modelStatus retrieves the model status for the specified entity.
func (c controller) modelStatus(ctx context.Context, arg jujuparams.Entity) (*jujuparams.ModelStatus, error) {
	mi, err := c.root.modelInfo(ctx, arg, false)
	if err != nil {
		return &jujuparams.ModelStatus{}, errgo.Mask(err, errgo.Is(params.ErrNotFound))
	}
	return &jujuparams.ModelStatus{
		ModelTag:           names.NewModelTag(mi.UUID).String(),
		Life:               mi.Life,
		HostedMachineCount: len(mi.Machines),
		ApplicationCount:   0,
		OwnerTag:           mi.OwnerTag,
		Machines:           mi.Machines,
	}, nil
}

// ControllerConfig returns the controller's configuration.
func (c controller) ControllerConfig() (jujuparams.ControllerConfigResult, error) {
	result := jujuparams.ControllerConfigResult{
		Config: map[string]interface{}{
			"charmstore-url": c.root.params.CharmstoreLocation,
			"metering-url":   c.root.params.MeteringLocation,
		},
	}
	return result, nil
}

// ModelConfig returns implements the controller facade's ModelConfig
// method. This always returns a permission error, as no user has admin
// access to the controller.
func (c controller) ModelConfig() (jujuparams.ModelConfigResults, error) {
	return jujuparams.ModelConfigResults{}, &jujuparams.Error{
		Code:    jujuparams.CodeUnauthorized,
		Message: "permission denied",
	}
}

// allModels returns all the models the logged in user has access to.
func (r *controllerRoot) allModels(ctx context.Context) (jujuparams.UserModelList, error) {
	var models []jujuparams.UserModel
	err := r.doModels(ctx, func(ctx context.Context, model *mongodoc.Model) error {
		models = append(models, jujuparams.UserModel{
			Model:          userModelForModelDoc(model),
			LastConnection: nil, // TODO (mhilton) work out how to record and set this.
		})
		return nil
	})
	if err != nil {
		return jujuparams.UserModelList{}, errgo.Mask(err)
	}
	return jujuparams.UserModelList{
		UserModels: models,
	}, nil
}

func userModelForModelDoc(m *mongodoc.Model) jujuparams.Model {
	return jujuparams.Model{
		Name:     string(m.Path.Name),
		UUID:     m.UUID,
		Type:     m.Type,
		OwnerTag: jem.UserTag(m.Path.User).String(),
	}
}

// modelInfo retrieves the model information for the specified entity.
func (r *controllerRoot) modelInfo(ctx context.Context, arg jujuparams.Entity, localOnly bool) (*jujuparams.ModelInfo, error) {
	model, err := getModel(ctx, r.jem, arg.Tag, auth.CheckCanRead)
	if err != nil {
		return nil, errgo.Mask(err,
			errgo.Is(params.ErrBadRequest),
			errgo.Is(params.ErrUnauthorized),
			errgo.Is(params.ErrNotFound),
		)
	}
	ctx = zapctx.WithFields(ctx, zap.String("model-uuid", model.UUID))
	info, err := r.modelDocToModelInfo(ctx, model)
	if err != nil {
		return nil, errgo.Mask(err)
	}
	if localOnly {
		return info, nil
	}
	// Query the model itself for user information.
	infoFromController, err := fetchModelInfo(ctx, r.jem, model)
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
	info.Users = filterUsers(ctx, infoFromController.Users, isModelAdmin(ctx, infoFromController))
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
	if auth.CheckIsAdmin(ctx, model) == nil {
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
		ut := userTag(auth.Username(ctx))
		users = append(users, jujuparams.ModelUserInfo{
			UserName:    ut.Id(),
			DisplayName: ut.Name(),
			Access:      userLevels[auth.Username(ctx)],
		})
	}
	return &jujuparams.ModelInfo{
		Name:               string(model.Path.Name),
		UUID:               model.UUID,
		ControllerUUID:     r.params.ControllerUUID,
		ProviderType:       providerType,
		DefaultSeries:      model.DefaultSeries,
		CloudTag:           jem.CloudTag(model.Cloud).String(),
		CloudRegion:        model.CloudRegion,
		CloudCredentialTag: jem.CloudCredentialTag(model.Credential).String(),
		OwnerTag:           jem.UserTag(model.Path.User).String(),
		Life:               life.Value(model.Life()),
		Status:             modelStatus(model.Info),
		Users:              users,
		Machines:           jemMachinesToModelMachineInfo(machines),
		AgentVersion:       modelVersion(ctx, model.Info),
		Type:               model.Type,
	}, nil
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
func isModelAdmin(ctx context.Context, info *jujuparams.ModelInfo) bool {
	var admin bool
	iterUsers(ctx, info.Users, func(u params.User, ui jujuparams.ModelUserInfo) {
		admin = admin || ui.Access == jujuparams.ModelAdminAccess && auth.CheckIsUser(ctx, u) == nil
	})
	return admin
}

// filterUsers returns a slice holding all of the given users that the
// current user should be able to see. Admin users can see everyone;
// other users can only see users and groups they're a member of. Users
// local to the controller are always removed.
func filterUsers(ctx context.Context, users []jujuparams.ModelUserInfo, admin bool) []jujuparams.ModelUserInfo {
	filtered := make([]jujuparams.ModelUserInfo, 0, len(users))
	iterUsers(ctx, users, func(u params.User, ui jujuparams.ModelUserInfo) {
		if admin || auth.CheckIsUser(ctx, u) == nil {
			filtered = append(filtered, ui)
		}
	})
	return filtered
}

// iterUsers iterates through all the non-local users in users and calls
// f with each in turn.
func iterUsers(ctx context.Context, users []jujuparams.ModelUserInfo, f func(params.User, jujuparams.ModelUserInfo)) {
	for _, u := range users {
		if !names.IsValidUser(u.UserName) {
			zapctx.Info(ctx, "controller sent invalid username, skipping", zap.String("username", u.UserName))
			continue
		}
		tag := names.NewUserTag(u.UserName)
		user, err := user(tag)
		if err != nil {
			// This error will occur if the user is local to
			// the controller, it can be safely ignored.
			continue
		}
		f(user, u)
	}
}

// jimm implements a facade containing JIMM-specific API calls.
type jimm struct {
	root *controllerRoot
}

// UserModelStats returns statistics about all the models that were created
// by the currently authenticated user.
func (j jimm) UserModelStats(ctx context.Context) (params.UserModelStatsResponse, error) {
	ctx = ctxutil.Join(ctx, j.root.authContext)
	models := make(map[string]params.ModelStats)

	user := auth.Username(ctx)
	it := j.root.jem.DB.NewCanReadIter(ctx,
		j.root.jem.DB.Models().
			Find(bson.D{{"creator", user}}).
			Select(bson.D{{"uuid", 1}, {"path", 1}, {"creator", 1}, {"counts", 1}}).
			Iter())
	var model mongodoc.Model
	for it.Next(&model) {
		models[model.UUID] = params.ModelStats{
			Model:  userModelForModelDoc(&model),
			Counts: model.Counts,
		}
	}
	if err := it.Err(); err != nil {
		return params.UserModelStatsResponse{}, errgo.Mask(err)
	}
	return params.UserModelStatsResponse{
		Models: models,
	}, nil
}

// getModel attempts to get the specified model from jem. If the model
// tag is not valid then the error cause will be params.ErrBadRequest. If
// the model cannot be found then the error cause will be
// params.ErrNotFound. If authf is non-nil then it will be called with
// the found model. authf is used to authenticate access to the model, if
// access is denied authf should return an error with the cause
// params.ErrUnauthorized. The cause of any error returned by authf will
// not be masked.
func getModel(ctx context.Context, jem *jem.JEM, modelTag string, authf func(context.Context, auth.ACLEntity) error) (*mongodoc.Model, error) {
	tag, err := names.ParseModelTag(modelTag)
	if err != nil {
		return nil, errgo.WithCausef(err, params.ErrBadRequest, "invalid model tag")
	}
	model, err := jem.DB.ModelFromUUID(ctx, tag.Id())
	if err != nil {
		return nil, errgo.Mask(err, errgo.Is(params.ErrNotFound))
	}
	if authf == nil {
		return model, nil
	}
	if err := authf(ctx, model); err != nil {
		return nil, errgo.Mask(err, errgo.Any)
	}
	if model.Cloud != "" {
		return model, nil
	}
	// The model does not currently store its cloud information so go
	// and fetch it from the model itself. This happens if the model
	// was created with a JIMM version older than 0.9.5.
	info, err := fetchModelInfo(ctx, jem, model)
	if err != nil {
		return nil, errgo.Mask(err)
	}
	cloudTag, err := names.ParseCloudTag(info.CloudTag)
	if err != nil {
		return nil, errgo.Notef(err, "bad data from controller")
	}
	credentialTag, err := names.ParseCloudCredentialTag(info.CloudCredentialTag)
	if err != nil {
		return nil, errgo.Notef(err, "bad data from controller")
	}
	model.Cloud = params.Cloud(cloudTag.Id())
	model.CloudRegion = info.CloudRegion
	owner, err := user(credentialTag.Owner())
	if err != nil {
		return nil, errgo.Mask(err, errgo.Is(params.ErrBadRequest))
	}
	model.Credential = params.CredentialPath{
		Cloud: params.Cloud(credentialTag.Cloud().Id()),
		User:  owner,
		Name:  params.CredentialName(credentialTag.Name()),
	}
	model.DefaultSeries = info.DefaultSeries

	if err := jem.DB.UpdateLegacyModel(ctx, model); err != nil {
		zapctx.Warn(ctx, "cannot update %s with cloud details", zap.String("model", model.Path.String()), zaputil.Error(err))
	}
	return model, nil
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

// pinger implements the Pinger facade.
type pinger struct{}

// Ping implements the Pinger facade's Ping method. It doesn't do
// anything.
func (p pinger) Ping() {}

// userManager implements the UserManager facade.
type userManager struct {
	root *controllerRoot
}

// AddUser implements the UserManager facade's AddUser method.
func (u userManager) AddUser(args jujuparams.AddUsers) (jujuparams.AddUserResults, error) {
	return jujuparams.AddUserResults{}, params.ErrUnauthorized
}

// RemoveUser implements the UserManager facade's RemoveUser method.
func (u userManager) RemoveUser(jujuparams.Entities) (jujuparams.ErrorResults, error) {
	return jujuparams.ErrorResults{}, params.ErrUnauthorized
}

// EnableUser implements the UserManager facade's EnableUser method.
func (u userManager) EnableUser(jujuparams.Entities) (jujuparams.ErrorResults, error) {
	return jujuparams.ErrorResults{}, params.ErrUnauthorized
}

// DisableUser implements the UserManager facade's DisableUser method.
func (u userManager) DisableUser(jujuparams.Entities) (jujuparams.ErrorResults, error) {
	return jujuparams.ErrorResults{}, params.ErrUnauthorized
}

// UserInfo implements the UserManager facade's UserInfo method.
func (u userManager) UserInfo(ctx context.Context, req jujuparams.UserInfoRequest) (jujuparams.UserInfoResults, error) {
	ctx = ctxutil.Join(ctx, u.root.authContext)
	res := jujuparams.UserInfoResults{
		Results: make([]jujuparams.UserInfoResult, len(req.Entities)),
	}
	for i, ent := range req.Entities {
		ui, err := u.userInfo(ctx, ent.Tag)
		if err != nil {
			res.Results[i].Error = mapError(err)
			continue
		}
		res.Results[i].Result = ui
	}
	return res, nil
}

func (u userManager) userInfo(ctx context.Context, entity string) (*jujuparams.UserInfo, error) {
	userTag, err := names.ParseUserTag(entity)
	if err != nil {
		return nil, errgo.WithCausef(err, params.ErrBadRequest, "invalid user tag")
	}
	user, err := user(userTag)
	if err != nil {
		return nil, errgo.Mask(err, errgo.Is(params.ErrBadRequest))
	}
	if auth.Username(ctx) != string(user) {
		return nil, params.ErrUnauthorized
	}
	return u.currentUser(ctx)
}

func (u userManager) currentUser(ctx context.Context) (*jujuparams.UserInfo, error) {
	userTag := userTag(auth.Username(ctx))
	return &jujuparams.UserInfo{
		// TODO(mhilton) a number of these fields should
		// be fetched from the identity manager, but that
		// will have to change to support getting them.
		Username:    userTag.Id(),
		DisplayName: userTag.Id(),
		Access:      string(permission.AddModelAccess),
		Disabled:    false,
	}, nil
}

// SetPassword implements the UserManager facade's SetPassword method.
func (u userManager) SetPassword(jujuparams.EntityPasswords) (jujuparams.ErrorResults, error) {
	return jujuparams.ErrorResults{}, params.ErrUnauthorized
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

// user creates a params.User from the given UserTag. If the UserTag is
// for a local user then an error will be returned. If the UserTag has
// the domain "external" then the returned User will only contain the
// name part.
func user(tag names.UserTag) (params.User, error) {
	if tag.IsLocal() {
		return "", errgo.WithCausef(nil, params.ErrBadRequest, "unsupported local user")
	}
	var username string
	if tag.Domain() == "external" {
		username = tag.Name()
	} else {
		username = tag.Id()
	}
	return params.User(username), nil
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

// authorizer implements facade.Authorizer
type authorizer struct {
	ctx context.Context
}

func (a authorizer) GetAuthTag() names.Tag {
	n := auth.Username(a.ctx)
	if names.IsValidUserName(n) {
		return names.NewLocalUserTag(n)
	}
	return names.NewUserTag(n)
}

func (authorizer) AuthController() bool {
	return false
}

func (authorizer) AuthMachineAgent() bool {
	return false
}

func (authorizer) AuthApplicationAgent() bool {
	return false
}

func (authorizer) AuthUnitAgent() bool {
	return false
}

func (a authorizer) AuthOwner(tag names.Tag) bool {
	t := a.GetAuthTag()
	return tag.Kind() == t.Kind() && tag.Id() == t.Id()
}

func (authorizer) AuthClient() bool {
	return true
}

func (authorizer) HasPermission(operation permission.Access, target names.Tag) (bool, error) {
	return false, nil
}

func (authorizer) UserHasPermission(user names.UserTag, operation permission.Access, target names.Tag) (bool, error) {
	return false, nil
}

func (authorizer) ConnectedModel() string {
	return ""
}
