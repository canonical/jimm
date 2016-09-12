// Copyright 2016 Canonical Ltd.

package jujuapi

import (
	"reflect"
	"sort"
	"strings"
	"time"

	modelmanagerapi "github.com/juju/juju/api/modelmanager"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/observer"
	jujuparams "github.com/juju/juju/apiserver/params"
	jujucloud "github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/network"
	"github.com/juju/juju/rpc"
	"github.com/juju/juju/rpc/jsoncodec"
	"github.com/juju/juju/rpc/rpcreflect"
	"github.com/juju/loggo"
	"golang.org/x/net/websocket"
	"gopkg.in/errgo.v1"
	"gopkg.in/juju/names.v2"
	"gopkg.in/macaroon-bakery.v1/bakery"
	"gopkg.in/macaroon-bakery.v1/bakery/checkers"
	"gopkg.in/mgo.v2/bson"

	"github.com/CanonicalLtd/jem/internal/jem"
	"github.com/CanonicalLtd/jem/internal/jemserver"
	"github.com/CanonicalLtd/jem/internal/mongodoc"
	"github.com/CanonicalLtd/jem/params"
)

var logger = loggo.GetLogger("jem.internal.jujuapi")

// mapError maps JEM errors to errors suitable for use with the juju API.
func mapError(err error) *jujuparams.Error {
	if err == nil {
		return nil
	}
	logger.Debugf("error: %s\n details: %s", err.Error(), errgo.Details(err))
	if perr, ok := err.(*jujuparams.Error); ok {
		return perr
	}
	msg := err.Error()
	code := ""
	switch errgo.Cause(err) {
	case params.ErrNotFound:
		code = jujuparams.CodeNotFound
	}
	return &jujuparams.Error{
		Message: msg,
		Code:    code,
	}
}

type facade struct {
	name    string
	version int
}

// heartMonitor is a interface that will monitor a connection and fail it
// if a heartbeat is not received within a certain time.
type heartMonitor interface {
	// Heartbeat signals to the HeartMonitor that the connection is still alive.
	Heartbeat()

	// Dead returns a channel that will be signalled if the heartbeat
	// is not detected quickly enough.
	Dead() <-chan time.Time

	// Stop stops the HeartMonitor from monitoring. It return true if
	// the connection is already dead when Stop was called.
	Stop() bool
}

// timerHeartMonitor implements heartMonitor using a standard time.Timer.
type timerHeartMonitor struct {
	*time.Timer
	duration time.Duration
}

// Heartbeat implements HeartMonitor.Heartbeat.
func (h timerHeartMonitor) Heartbeat() {
	h.Timer.Reset(h.duration)
}

// Dead implements HeartMonitor.Dead.
func (h timerHeartMonitor) Dead() <-chan time.Time {
	return h.Timer.C
}

// newHeartMonitor is defined as a variable so that it can be overriden in tests.
var newHeartMonitor = func(d time.Duration) heartMonitor {
	return timerHeartMonitor{
		Timer:    time.NewTimer(d),
		duration: d,
	}
}

// facades contains the list of facade versions supported by this API.
var facades = map[facade]string{
	facade{"Admin", 3}:        "Admin",
	facade{"Cloud", 1}:        "Cloud",
	facade{"ModelManager", 2}: "ModelManager",
	facade{"Pinger", 1}:       "Pinger",
}

// newWSServer creates a new WebSocket server suitible for handling the API for modelUUID.
func newWSServer(jem *jem.JEM, jsParams jemserver.Params, modelUUID string) websocket.Server {
	hnd := wsHandler{
		jem:           jem,
		params:        jsParams,
		modelUUID:     modelUUID,
		schemataCache: make(map[params.Cloud]map[jujucloud.AuthType]jujucloud.CredentialSchema),
	}
	return websocket.Server{
		Handler: hnd.handle,
	}
}

// wsHandler is a handler for a particular WebSocket connection.
type wsHandler struct {
	jem           *jem.JEM
	params        jemserver.Params
	heartMonitor  heartMonitor
	modelUUID     string
	conn          *rpc.Conn
	model         *mongodoc.Model
	controller    *mongodoc.Controller
	schemataCache map[params.Cloud]map[jujucloud.AuthType]jujucloud.CredentialSchema
}

// handle handles the connection.
func (h *wsHandler) handle(wsConn *websocket.Conn) {
	codec := jsoncodec.NewWebsocket(wsConn)
	h.conn = rpc.NewConn(codec, observer.None())

	h.conn.ServeRoot(h, func(err error) error {
		return mapError(err)
	})
	h.heartMonitor = newHeartMonitor(h.params.WebsocketPingTimeout)
	h.conn.Start()
	select {
	case <-h.heartMonitor.Dead():
		logger.Infof("PING Timeout")
	case <-h.conn.Dead():
		h.heartMonitor.Stop()
	}
	h.conn.Close()
}

// resolveUUID finds the JEM model from the UUID that was specified in
// the URL path and sets h.model and h.controller appropriately from
// that.
func (h *wsHandler) resolveUUID() error {
	if h.modelUUID == "" {
		return nil
	}
	var err error
	h.model, err = h.jem.DB.ModelFromUUID(h.modelUUID)
	if err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrNotFound))
	}
	h.controller, err = h.jem.DB.Controller(h.model.Controller)
	return errgo.Mask(err)
}

// credentialSchema gets the schema for the credential identified by the
// given cloud and authType.
func (h *wsHandler) credentialSchema(cloud params.Cloud, authType string) (jujucloud.CredentialSchema, error) {
	if cs, ok := h.schemataCache[cloud]; ok {
		return cs[jujucloud.AuthType(authType)], nil
	}
	cloudInfo, err := h.jem.DB.Cloud(cloud)
	if err != nil {
		return nil, errgo.Mask(err, errgo.Is(params.ErrNotFound))
	}
	provider, err := environs.Provider(cloudInfo.ProviderType)
	if err != nil {
		return nil, errgo.Mask(err)
	}
	h.schemataCache[cloud] = provider.CredentialSchemas()
	return h.schemataCache[cloud][jujucloud.AuthType(authType)], nil
}

// FindMethod implements rpcreflect.MethodFinder.
func (h *wsHandler) FindMethod(rootName string, version int, methodName string) (rpcreflect.MethodCaller, error) {
	if h.model == nil || h.controller == nil {
		if err := h.resolveUUID(); err != nil {
			return nil, errgo.Mask(err)
		}
	}
	if h.jem.Auth.Username == "" && rootName != "Admin" {
		return nil, &rpcreflect.CallNotImplementedError{
			RootMethod: rootName,
			Version:    version,
		}
	}
	if rootName == "Admin" && version < 3 {
		return nil, &rpc.RequestError{
			Code:    jujuparams.CodeNotSupported,
			Message: "JAAS does not support login from old clients",
		}
	}

	if rn := facades[facade{rootName, version}]; rn != "" {
		// TODO(rogpeppe) avoid doing all this reflect code on every RPC call.
		return rpcreflect.ValueOf(reflect.ValueOf(root{h})).FindMethod(rn, 0, methodName)
	}

	return nil, &rpcreflect.CallNotImplementedError{
		RootMethod: rootName,
		Version:    version,
	}
}

// Kill implements rpcreflect.Root.Kill; it does nothing because there
// are no long-running requests that need to be killed. If we add a
// watcher, this will need to be changed.
func (h *wsHandler) Kill() {
}

// root contains the root of the api handlers.
type root struct {
	h *wsHandler
}

// Admin returns an implementation of the Admin facade (version 3).
func (r root) Admin(id string) (admin, error) {
	if id != "" {
		// Safeguard id for possible future use.
		return admin{}, common.ErrBadId
	}
	return admin{r.h}, nil
}

// Cloud returns an implementation of the Cloud facade (version 1).
func (r root) Cloud(id string) (cloud, error) {
	if id != "" {
		// Safeguard id for possible future use.
		return cloud{}, common.ErrBadId
	}
	return cloud{r.h}, nil
}

// ModelManager returns an implementation of the ModelManager facade
// (version 2).
func (r root) ModelManager(id string) (modelManager, error) {
	if id != "" {
		// Safeguard id for possible future use.
		return modelManager{}, common.ErrBadId
	}
	return modelManager{r.h}, nil
}

// Pinger returns an implementation of the Pinger facade
// (version 1).
func (r root) Pinger(id string) (pinger, error) {
	if id != "" {
		// Safeguard id for possible future use.
		return pinger{}, common.ErrBadId
	}
	return pinger{r.h}, nil
}

// admin implements the Admin facade.
type admin struct {
	h *wsHandler
}

// Login implements the Login method on the Admin facade.
func (a admin) Login(req jujuparams.LoginRequest) (jujuparams.LoginResult, error) {
	if a.h.modelUUID != "" {
		if a.h.model.Id == a.h.controller.Id {
			// The client cannot log in to a controller model via JIMM.
			return jujuparams.LoginResult{}, errgo.WithCausef(nil, params.ErrNotFound, "model %q not found", a.h.modelUUID)
		}
		// If the connection specifies a model then redirection is required.
		return jujuparams.LoginResult{}, &jujuparams.Error{
			Code:    jujuparams.CodeRedirect,
			Message: "redirection required",
		}
	}

	// JAAS only supports macaroon login, ignore all the other fields.
	attr, err := a.h.jem.Bakery.CheckAny(req.Macaroons, nil, checkers.TimeBefore)
	if err != nil {
		if verr, ok := errgo.Cause(err).(*bakery.VerificationError); ok {
			m, err := a.h.jem.NewMacaroon()
			if err != nil {
				return jujuparams.LoginResult{}, errgo.Notef(err, "cannot create macaroon")
			}
			return jujuparams.LoginResult{
				DischargeRequired:       m,
				DischargeRequiredReason: verr.Error(),
			}, nil
		}
		return jujuparams.LoginResult{}, errgo.Mask(err)
	}
	a.h.jem.Auth.Username = attr["username"]

	return jujuparams.LoginResult{
		UserInfo: &jujuparams.AuthUserInfo{
			DisplayName: a.h.jem.Auth.Username,
			Identity:    names.NewUserTag(a.h.jem.Auth.Username).WithDomain("external").String(),
		},
		ControllerTag: names.NewControllerTag(a.h.params.ControllerUUID).String(),
		Facades:       facadeVersions(),
		ServerVersion: "2.0.0",
	}, nil
}

// facadeVersions creates a list of facadeVersions as specified in
// facades.
func facadeVersions() []jujuparams.FacadeVersions {
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

// RedirectInfo implements the RedirectInfo method on the Admin facade.
func (a admin) RedirectInfo() (jujuparams.RedirectInfoResult, error) {
	if a.h.modelUUID == "" {
		return jujuparams.RedirectInfoResult{}, errgo.New("not redirected")
	}
	nhps, err := network.ParseHostPorts(a.h.controller.HostPorts...)
	if err != nil {
		return jujuparams.RedirectInfoResult{}, errgo.Mask(err)
	}
	hps := jujuparams.FromNetworkHostPorts(nhps)
	return jujuparams.RedirectInfoResult{
		Servers: [][]jujuparams.HostPort{hps},
		CACert:  a.h.controller.CACert,
	}, nil
}

// cloud implements the Cloud facade.
type cloud struct {
	h *wsHandler
}

// Cloud implements the Cloud method of the Cloud facade.
func (c cloud) Cloud(ents jujuparams.Entities) (jujuparams.CloudResults, error) {
	cloudResults := make([]jujuparams.CloudResult, len(ents.Entities))
	clouds, err := c.clouds()
	if err != nil {
		return jujuparams.CloudResults{}, mapError(err)
	}
	for i, ent := range ents.Entities {
		cloud, err := c.cloud(ent.Tag, clouds)
		if err != nil {
			cloudResults[i].Error = mapError(err)
			continue
		}
		cloudResults[i].Cloud = cloud
	}
	return jujuparams.CloudResults{
		Results: cloudResults,
	}, nil
}

// cloud finds and returns the cloud identified by cloudTag in clouds.
func (c cloud) cloud(cloudTag string, clouds map[string]jujuparams.Cloud) (*jujuparams.Cloud, error) {
	if cloud, ok := clouds[cloudTag]; ok {
		return &cloud, nil
	}
	ct, err := names.ParseCloudTag(cloudTag)
	if err != nil {
		return nil, errgo.WithCausef(err, params.ErrBadRequest, "")
	}
	return nil, errgo.WithCausef(nil, params.ErrNotFound, "cloud %q not available", ct.Id())
}

// Clouds implements the Clouds method on the Cloud facade.
func (c cloud) Clouds() (jujuparams.CloudsResult, error) {
	var res jujuparams.CloudsResult
	var err error
	res.Clouds, err = c.clouds()
	return res, errgo.Mask(err)
}

func (c cloud) clouds() (map[string]jujuparams.Cloud, error) {
	clouds := make(map[string]jujuparams.Cloud)

	err := c.h.jem.DoControllers("", "", func(ctl *mongodoc.Controller) error {
		cloudTag := jem.CloudTag(ctl.Cloud.Name).String()
		// TODO consider caching this result because it will be often called and
		// the result will change very rarely.
		clouds[cloudTag] = mergeClouds(clouds[cloudTag], makeCloud(ctl.Cloud))
		return nil
	})
	if err != nil {
		return nil, errgo.Mask(err)
	}
	return clouds, nil
}

// DefaultCloud implements the DefaultCloud method of the Cloud facade.
func (c cloud) DefaultCloud() (jujuparams.StringResult, error) {
	return jujuparams.StringResult{
		Result: names.NewCloudTag(c.h.params.DefaultCloud).String(),
	}, nil
}

// UserCredentials implements the UserCredentials method of the Cloud facade.
func (c cloud) UserCredentials(userclouds jujuparams.UserClouds) (jujuparams.StringsResults, error) {
	results := make([]jujuparams.StringsResult, len(userclouds.UserClouds))
	for i, ent := range userclouds.UserClouds {
		creds, err := c.userCredentials(ent.UserTag, ent.CloudTag)
		if err != nil {
			results[i].Error = mapError(err)
			continue
		}
		results[i].Result = creds
	}

	return jujuparams.StringsResults{
		Results: results,
	}, nil
}

// userCredentials retrieves the credentials stored for given owner and cloud.
func (c cloud) userCredentials(ownerTag, cloudTag string) ([]string, error) {
	owner, err := names.ParseUserTag(ownerTag)
	if err != nil {
		return nil, errgo.WithCausef(err, params.ErrBadRequest, "")

	}
	if owner.IsLocal() {
		return nil, errgo.WithCausef(nil, params.ErrBadRequest, "unsupported domain %q", owner.Domain())
	}
	cld, err := names.ParseCloudTag(cloudTag)
	if err != nil {
		return nil, errgo.WithCausef(err, params.ErrBadRequest, "")

	}
	var cloudCreds []string
	it := c.h.jem.CanReadIter(c.h.jem.DB.Credentials().Find(bson.D{{"user", owner.Name()}, {"cloud", cld.Id()}}).Iter())
	var cred mongodoc.Credential
	for it.Next(&cred) {
		cloudCreds = append(cloudCreds, jem.CloudCredentialTag(cred.Cloud, cred.User, cred.Name).String())
	}

	return cloudCreds, errgo.Mask(it.Err())
}

// UpdateCredentials implements the UpdateCredentials method of the Cloud
// facade.
func (c cloud) UpdateCredentials(args jujuparams.UpdateCloudCredentials) (jujuparams.ErrorResults, error) {
	results := make([]jujuparams.ErrorResult, len(args.Credentials))
	for i, ucc := range args.Credentials {
		if err := c.updateCredential(ucc); err != nil {
			results[i].Error = mapError(err)
		}
	}
	return jujuparams.ErrorResults{
		Results: results,
	}, nil
}

// updateCredential adds a single credential to the database.
func (c cloud) updateCredential(arg jujuparams.UpdateCloudCredential) error {
	tag, err := names.ParseCloudCredentialTag(arg.Tag)
	if err != nil {
		return errgo.WithCausef(err, params.ErrBadRequest, "")
	}
	ownerTag := tag.Owner()
	if ownerTag.IsLocal() {
		return errgo.WithCausef(nil, params.ErrBadRequest, "unsupported domain %q", ownerTag.Domain())
	}
	var owner params.User
	if err := owner.UnmarshalText([]byte(ownerTag.Name())); err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrBadRequest))
	}
	if err := c.h.jem.CheckIsUser(owner); err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrUnauthorized))
	}
	var name params.Name
	if err := name.UnmarshalText([]byte(tag.Name())); err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrBadRequest))
	}
	credential := mongodoc.Credential{
		User:       owner,
		Cloud:      params.Cloud(tag.Cloud().Id()),
		Name:       params.Name(tag.Name()),
		Type:       arg.Credential.AuthType,
		Attributes: arg.Credential.Attributes,
	}
	if err := c.h.jem.UpdateCredential(&credential); err != nil {
		return errgo.Mask(err)
	}
	return nil
}

// Credential implements the Credential method of the Cloud facade.
func (c cloud) Credential(args jujuparams.Entities) (jujuparams.CloudCredentialResults, error) {
	results := make([]jujuparams.CloudCredentialResult, len(args.Entities))
	for i, e := range args.Entities {
		cred, err := c.credential(e.Tag)
		if err != nil {
			results[i].Error = mapError(err)
			continue
		}
		results[i].Result = cred
	}
	return jujuparams.CloudCredentialResults{
		Results: results,
	}, nil
}

// credential retrieves the given credential.
func (c cloud) credential(cloudCredentialTag string) (*jujuparams.CloudCredential, error) {
	cct, err := names.ParseCloudCredentialTag(cloudCredentialTag)
	if err != nil {
		return nil, errgo.WithCausef(err, params.ErrBadRequest, "")

	}
	owner := cct.Owner()
	if owner.IsLocal() {
		return nil, errgo.WithCausef(nil, params.ErrNotFound, "credential %q not found", cct.Id())
	}
	if err := c.h.jem.CheckIsUser(params.User(owner.Name())); err != nil {
		return nil, errgo.Mask(err, errgo.Is(params.ErrUnauthorized))
	}
	cred, err := c.h.jem.DB.Credential(params.User(owner.Name()), params.Cloud(cct.Cloud().Id()), params.Name(cct.Name()))
	if err != nil {
		return nil, errgo.Mask(err, errgo.Is(params.ErrNotFound))
	}
	schema, err := c.h.credentialSchema(cred.Cloud, cred.Type)
	if err != nil {
		return nil, errgo.Mask(err, errgo.Is(params.ErrNotFound))
	}
	cc := jujuparams.CloudCredential{
		AuthType:   cred.Type,
		Attributes: make(map[string]string),
	}
	for k, v := range cred.Attributes {
		if ca, ok := schema.Attribute(k); ok && !ca.Hidden {
			cc.Attributes[k] = v
		} else {
			cc.Redacted = append(cc.Redacted, k)
		}
	}
	return &cc, nil
}

// modelManager implements the ModelManager facade.
type modelManager struct {
	h *wsHandler
}

// ListModels returns the models that the authenticated user
// has access to. The user parameter is ignored.
func (m modelManager) ListModels(_ jujuparams.Entity) (jujuparams.UserModelList, error) {
	var models []jujuparams.UserModel

	it := m.h.jem.CanReadIter(m.h.jem.DB.Models().Find(nil).Sort("_id").Iter())

	var model mongodoc.Model
	for it.Next(&model) {
		models = append(models, jujuparams.UserModel{
			Model: jujuparams.Model{
				Name:     string(model.Path.Name),
				UUID:     model.UUID,
				OwnerTag: jem.UserTag(model.Path.User).String(),
			},
			LastConnection: nil, // TODO (mhilton) work out how to record and set this.
		})
	}
	if err := it.Err(); err != nil {
		return jujuparams.UserModelList{}, errgo.Mask(err)
	}
	return jujuparams.UserModelList{
		UserModels: models,
	}, nil
}

// ModelInfo implements the ModelManager facade's ModelInfo method.
func (m modelManager) ModelInfo(args jujuparams.Entities) (jujuparams.ModelInfoResults, error) {
	results := make([]jujuparams.ModelInfoResult, len(args.Entities))

	// TODO (mhilton) get information for all of the models connected
	// to a single controller in one go.
	for i, arg := range args.Entities {
		mi, err := m.modelInfo(arg)
		if err != nil {
			results[i].Error = mapError(err)
			continue
		}
		results[i].Result = mi
	}

	return jujuparams.ModelInfoResults{
		Results: results,
	}, nil
}

// modelInfo retrieves the model information for the specified entity.
func (m modelManager) modelInfo(arg jujuparams.Entity) (*jujuparams.ModelInfo, error) {
	tag, model, err := m.getModel(arg.Tag, m.h.jem.CheckCanRead)
	if err != nil {
		return nil, errgo.Mask(err, errgo.Is(params.ErrBadRequest), errgo.Is(params.ErrUnauthorized))
	}
	conn, err := m.h.jem.OpenAPI(model.Controller)
	if err != nil {
		return nil, errgo.Mask(err)
	}
	defer conn.Close()
	client := modelmanagerapi.NewClient(conn)
	mirs, err := client.ModelInfo([]names.ModelTag{tag})
	if err != nil {
		return nil, errgo.Mask(err)
	}
	if mirs[0].Error != nil {
		return nil, errgo.Mask(mirs[0].Error)
	}
	mi1 := m.massageModelInfo(*mirs[0].Result)
	return &mi1, nil
}

// massageModelInfo modifies the modelInfo returned from a controller as
// if it was returned from the jimm controller.
func (m modelManager) massageModelInfo(mi jujuparams.ModelInfo) jujuparams.ModelInfo {
	mi1 := mi
	mi1.ControllerUUID = m.h.params.ControllerUUID
	mi1.Users = make([]jujuparams.ModelUserInfo, 0, len(mi.Users))
	for _, u := range mi.Users {
		if strings.HasSuffix(u.UserName, "@local") {
			continue
		}
		mi1.Users = append(mi1.Users, u)
	}
	return mi1
}

// CreateModel implements the ModelManager facade's CreateModel method.
func (m modelManager) CreateModel(args jujuparams.ModelCreateArgs) (jujuparams.ModelInfo, error) {
	owner, err := names.ParseUserTag(args.OwnerTag)
	if err != nil {
		return jujuparams.ModelInfo{}, errgo.WithCausef(err, params.ErrBadRequest, "invalid owner tag")
	}
	logger.Debugf("Attempting to create %s/%s", owner, args.Name)
	if owner.IsLocal() {
		return jujuparams.ModelInfo{}, params.ErrUnauthorized
	}
	if err := m.h.jem.CheckIsUser(params.User(owner.Name())); err != nil {
		return jujuparams.ModelInfo{}, errgo.Mask(err, errgo.Is(params.ErrUnauthorized))
	}
	cloud := params.Cloud(m.h.params.DefaultCloud)
	if args.CloudTag != "" {
		cloudTag, err := names.ParseCloudTag(args.CloudTag)
		if err != nil {
			return jujuparams.ModelInfo{}, errgo.WithCausef(err, params.ErrBadRequest, "invalid cloud tag")
		}
		cloud = params.Cloud(cloudTag.Id())
	}
	cloudCredentialTag, err := names.ParseCloudCredentialTag(args.CloudCredentialTag)
	if err != nil {
		return jujuparams.ModelInfo{}, errgo.WithCausef(err, params.ErrBadRequest, "invalid cloud credential tag")
	}

	ctlPath, cloud, region, err := m.h.jem.SelectController(cloud, args.CloudRegion)
	if err != nil {
		return jujuparams.ModelInfo{}, errgo.Mask(err, errgo.Is(params.ErrBadRequest), errgo.Is(params.ErrNotFound))
	}

	conn, err := m.h.jem.OpenAPI(ctlPath)
	if err != nil {
		return jujuparams.ModelInfo{}, errgo.Mask(err)
	}
	defer conn.Close()

	_, mi, err := m.h.jem.CreateModel(conn, jem.CreateModelParams{
		Path:           params.EntityPath{User: params.User(owner.Name()), Name: params.Name(args.Name)},
		ControllerPath: ctlPath,
		Credential:     params.Name(cloudCredentialTag.Name()),
		Cloud:          cloud,
		Region:         region,
		Attributes:     args.Config,
	})
	if err != nil {
		return jujuparams.ModelInfo{}, errgo.Mask(err, errgo.Is(params.ErrBadRequest), errgo.Is(params.ErrNotFound))
	}
	return m.massageModelInfo(*mi), nil
}

// DestroyModels implements the ModelManager facade's DestroyModels method.
func (m modelManager) DestroyModels(args jujuparams.Entities) (jujuparams.ErrorResults, error) {
	results := make([]jujuparams.ErrorResult, len(args.Entities))

	for i, arg := range args.Entities {
		if err := m.destroyModel(arg); err != nil {
			results[i].Error = mapError(err)
		}
	}

	return jujuparams.ErrorResults{
		Results: results,
	}, nil
}

// destroyModel destroys the specified model.
func (m modelManager) destroyModel(arg jujuparams.Entity) error {
	_, model, err := m.getModel(arg.Tag, m.checkIsOwner)
	if err != nil {
		if errgo.Cause(err) == params.ErrNotFound {
			// Juju doesn't treat removing a model that isn't there as an error, and neither should we.
			return nil
		}
		return errgo.Mask(err, errgo.Is(params.ErrBadRequest), errgo.Is(params.ErrUnauthorized))
	}
	conn, err := m.h.jem.OpenAPI(model.Controller)
	if err != nil {
		return errgo.Mask(err)
	}
	if err := m.h.jem.DestroyModel(conn, model); err != nil {
		return errgo.Mask(err)
	}
	return nil
}

// ModifyModelAccess implements the ModelManager facade's ModifyModelAccess method.
func (m modelManager) ModifyModelAccess(args jujuparams.ModifyModelAccessRequest) (jujuparams.ErrorResults, error) {
	results := make([]jujuparams.ErrorResult, len(args.Changes))
	for i, change := range args.Changes {
		err := m.modifyModelAccess(change)
		if err != nil {
			results[i].Error = mapError(err)
		}
	}
	return jujuparams.ErrorResults{
		Results: results,
	}, nil
}

func (m modelManager) modifyModelAccess(change jujuparams.ModifyModelAccess) error {
	_, model, err := m.getModel(change.ModelTag, m.checkIsOwner)
	if err != nil {
		if errgo.Cause(err) == params.ErrNotFound {
			err = params.ErrUnauthorized
		}
		return errgo.Mask(err, errgo.Is(params.ErrBadRequest), errgo.Is(params.ErrUnauthorized))
	}
	userTag, err := names.ParseUserTag(change.UserTag)
	if err != nil {
		return errgo.WithCausef(err, params.ErrBadRequest, "invalid user tag")
	}
	if userTag.IsLocal() {
		return errgo.WithCausef(nil, params.ErrBadRequest, "unsupported domain %q", userTag.Domain())
	}
	conn, err := m.h.jem.OpenAPI(model.Controller)
	if err != nil {
		return errgo.Mask(err)
	}
	defer conn.Close()
	switch change.Action {
	case jujuparams.GrantModelAccess:
		err = m.h.jem.GrantModel(conn, model, params.User(userTag.Name()), string(change.Access))
	case jujuparams.RevokeModelAccess:
		err = m.h.jem.RevokeModel(conn, model, params.User(userTag.Name()), string(change.Access))
	default:
		return errgo.WithCausef(err, params.ErrBadRequest, "invalid action %q", change.Action)
	}
	if err != nil {
		return errgo.Mask(err)
	}
	return nil
}

// checkIsOwner checks if the current user is the owner of the specified
// document.
func (m modelManager) checkIsOwner(e jem.ACLEntity) error {
	return errgo.Mask(m.h.jem.CheckIsUser(e.Owner()), errgo.Is(params.ErrUnauthorized))
}

// getModel attempts to get the specified model from jem. If the model
// tag is not valid then the error cause will be params.ErrBadRequest. If
// the model cannot be found then the error cause will be
// params.ErrNotFound. If authf is non-nil then it will be called with
// the found model. authf is used to authenticate access to the model, if
// access is denied authf should return an error with the cause
// params.ErrUnauthorized. The cause of any error returned by authf will
// not be masked.
func (m modelManager) getModel(modelTag string, authf func(jem.ACLEntity) error) (names.ModelTag, *mongodoc.Model, error) {
	tag, err := names.ParseModelTag(modelTag)
	if err != nil {
		return names.ModelTag{}, nil, errgo.WithCausef(err, params.ErrBadRequest, "invalid model tag")
	}
	model, err := m.h.jem.DB.ModelFromUUID(tag.Id())
	if err != nil {
		return names.ModelTag{}, nil, errgo.Mask(err, errgo.Is(params.ErrNotFound))
	}
	if authf == nil {
		return names.ModelTag{}, model, nil
	}
	if err := authf(model); err != nil {
		return names.ModelTag{}, nil, errgo.Mask(err, errgo.Any)
	}
	return tag, model, nil
}

// pinger implements the Pinger facade.
type pinger struct {
	h *wsHandler
}

// Ping implements the Pinger facade's Ping method.
func (p pinger) Ping() {
	p.h.heartMonitor.Heartbeat()
}
