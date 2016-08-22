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
func newWSServer(jem *jem.JEM, params jemserver.Params, modelUUID string) websocket.Server {
	hnd := wsHandler{
		jem:       jem,
		params:    params,
		modelUUID: modelUUID,
	}
	return websocket.Server{
		Handler: hnd.handle,
	}
}

// wsHandler is a handler for a particular WebSocket connection.
type wsHandler struct {
	jem          *jem.JEM
	params       jemserver.Params
	heartMonitor heartMonitor
	modelUUID    string
	conn         *rpc.Conn
	model        *mongodoc.Model
	controller   *mongodoc.Controller
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
	h.model, err = h.jem.ModelFromUUID(h.modelUUID)
	if err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrNotFound))
	}
	h.controller, err = h.jem.Controller(h.model.Controller)
	return errgo.Mask(err)
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
func (a admin) Login(req jujuparams.LoginRequest) (jujuparams.LoginResultV1, error) {
	if a.h.modelUUID != "" {
		if a.h.model.Id == a.h.controller.Id {
			// The client cannot log in to a controller model via JIMM.
			return jujuparams.LoginResultV1{}, errgo.WithCausef(nil, params.ErrNotFound, "model %q not found", a.h.modelUUID)
		}
		// If the connection specifies a model then redirection is required.
		return jujuparams.LoginResultV1{}, &jujuparams.Error{
			Code:    jujuparams.CodeRedirect,
			Message: "redirection required",
		}
	}

	// JAAS only supports macaroon login, ignore all the other fields.
	attr, err := a.h.jem.Bakery.CheckAny(req.Macaroons, nil, checkers.TimeBefore)
	if err != nil {
		if verr, ok := err.(*bakery.VerificationError); ok {
			m, err := a.h.jem.NewMacaroon()
			if err != nil {
				return jujuparams.LoginResultV1{}, errgo.Notef(err, "cannot create macaroon")
			}
			return jujuparams.LoginResultV1{
				DischargeRequired:       m,
				DischargeRequiredReason: verr.Error(),
			}, nil
		}
		return jujuparams.LoginResultV1{}, errgo.Mask(err)
	}
	a.h.jem.Auth.Username = attr["username"]

	return jujuparams.LoginResultV1{
		UserInfo: &jujuparams.AuthUserInfo{
			DisplayName: a.h.jem.Auth.Username,
			Identity:    names.NewUserTag(a.h.jem.Auth.Username).WithDomain("external").String(),
		},
		ControllerTag: names.NewModelTag(a.h.params.ControllerUUID).String(),
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
	for i, ent := range ents.Entities {
		cloudTag, err := names.ParseCloudTag(ent.Tag)
		if err != nil {
			cloudResults[i].Error = mapError(err)
			continue
		}
		cloudInfo, err := c.cloud(cloudTag)
		if err != nil {
			cloudResults[i].Error = mapError(err)
			continue
		}
		cloudResults[i].Cloud = cloudInfo
	}
	return jujuparams.CloudResults{
		Results: cloudResults,
	}, nil
}

func (c cloud) cloud(cloudTag names.CloudTag) (*jujuparams.Cloud, error) {
	// TODO(mhilton) maybe do something different when connected to a controller model
	var cloudInfo jujuparams.Cloud
	err := c.h.jem.DoControllers(params.Cloud(cloudTag.Id()), "", func(cnt *mongodoc.Controller) error {
		cloudInfo.Type = cnt.Cloud.ProviderType
		cloudInfo.AuthTypes = cnt.Cloud.AuthTypes
		// TODO (mhilton) fill out other fields
		for _, reg := range cnt.Cloud.Regions {
			cloudInfo.Regions = append(cloudInfo.Regions, jujuparams.CloudRegion{
				Name: reg.Name,
			})
		}
		return nil
	})
	if err != nil {
		return nil, errgo.Mask(err)
	}
	if cloudInfo.Type == "" {
		return nil, errgo.WithCausef(nil, params.ErrNotFound, "cloud %q not available", cloudTag.Id())
	}
	// TODO (mhilton) ensure list of regions is deterministic.
	return &cloudInfo, nil
}

// DefaultCloud implements the DefaultCloud method of the Cloud facade.
func (c cloud) DefaultCloud() (jujuparams.StringResult, error) {
	return jujuparams.StringResult{
		Result: names.NewCloudTag(c.h.params.DefaultCloud).String(),
	}, nil
}

// Credentials implements the Credentials method of the Cloud facade.
func (c cloud) Credentials(userclouds jujuparams.UserClouds) (jujuparams.StringsResults, error) {
	results := make([]jujuparams.StringsResult, len(userclouds.UserClouds))
	for i, ent := range userclouds.UserClouds {
		creds, err := c.credentials(ent.UserTag, ent.CloudTag)
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

// credentials retrieves the credentials stored for given owner and cloud.
func (c cloud) credentials(ownerTag, cloudTag string) ([]string, error) {
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
	tag, err := names.ParseModelTag(arg.Tag)
	if err != nil {
		return nil, errgo.WithCausef(err, params.ErrBadRequest, "")
	}
	model, err := m.h.jem.ModelFromUUID(tag.Id())
	if err != nil {
		if errgo.Cause(err) == params.ErrNotFound {
			return nil, errgo.WithCausef(err, params.ErrUnauthorized, "")
		}
		return nil, errgo.Mask(err)
	}
	if err := m.h.jem.CheckCanRead(model); err != nil {
		return nil, errgo.Mask(err, errgo.Is(params.ErrUnauthorized))
	}
	conn, err := m.h.jem.OpenAPI(model.Path)
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
	cloudCredentialTag, err := names.ParseCloudCredentialTag(args.CloudCredentialTag)
	if err != nil {
		return jujuparams.ModelInfo{}, errgo.WithCausef(err, params.ErrBadRequest, "invalid cloud credential tag")
	}

	ctlPath, cloud, region, err := m.h.jem.SelectController(params.Cloud(m.h.params.DefaultCloud), args.CloudRegion)
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
	modelTag, err := names.ParseModelTag(change.ModelTag)
	if err != nil {
		return errgo.WithCausef(err, params.ErrBadRequest, "invalid model tag")
	}
	model, err := m.h.jem.ModelFromUUID(modelTag.Id())
	if err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrNotFound))
	}
	if err := m.h.jem.CheckIsUser(model.Path.User); err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrUnauthorized))
	}
	userTag, err := names.ParseUserTag(change.UserTag)
	if err != nil {
		return errgo.WithCausef(err, params.ErrBadRequest, "invalid user tag")
	}
	if userTag.IsLocal() {
		return errgo.WithCausef(nil, params.ErrBadRequest, "unsupported domain %q", userTag.Domain())
	}
	controller, err := m.h.jem.Controller(model.Controller)
	if err != nil {
		return errgo.Mask(err)
	}
	conn, err := m.h.jem.OpenAPIFromDocs(model, controller)
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

// pinger implements the Pinger facade.
type pinger struct {
	h *wsHandler
}

// Ping implements the Pinger facade's Ping method.
func (p pinger) Ping() {
	p.h.heartMonitor.Heartbeat()
}
