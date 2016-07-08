// Copyright 2016 Canonical Ltd.

package jujuapi

import (
	"reflect"

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
	"github.com/CanonicalLtd/jem/internal/mongodoc"
	"github.com/CanonicalLtd/jem/params"
)

var logger = loggo.GetLogger("jem.internal.jujuapi")

// mapError maps JEM errors to errors suitable for use with the juju API.
func mapError(err error) error {
	if err == nil {
		return nil
	}
	logger.Debugf("error: %s\n details: %s", err.Error(), errgo.Details(err))
	if _, ok := err.(*jujuparams.Error); ok {
		return err
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

// newWSServer creates a new WebSocket server suitible for handling the API for modelUUID.
func newWSServer(jem *jem.JEM, modelUUID string) websocket.Server {
	hnd := wsHandler{
		jem:       jem,
		modelUUID: modelUUID,
	}
	return websocket.Server{
		Handler: hnd.handle,
	}
}

// wsHandler is a handler for a particular WebSocket connection.
type wsHandler struct {
	jem        *jem.JEM
	modelUUID  string
	conn       *rpc.Conn
	model      *mongodoc.Model
	controller *mongodoc.Controller
}

// handle handles the connection.
func (h *wsHandler) handle(wsConn *websocket.Conn) {
	codec := jsoncodec.NewWebsocket(wsConn)
	h.conn = rpc.NewConn(codec, observer.None())

	h.conn.ServeFinder(h, mapError)
	h.conn.Start()
	select {
	case <-h.conn.Dead():
	}
	h.conn.Close()
}

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

	switch {
	case rootName == "Admin" && version == 3:
		return rpcreflect.ValueOf(reflect.ValueOf(adminRoot{h})).FindMethod("Admin", 0, methodName)
	case rootName == "Cloud" && version == 1:
		return rpcreflect.ValueOf(reflect.ValueOf(cloudRoot{h})).FindMethod("Cloud", 0, methodName)
	}

	return nil, &rpcreflect.CallNotImplementedError{
		RootMethod: rootName,
		Version:    version,
	}
}

type adminRoot struct {
	h *wsHandler
}

func (a adminRoot) Admin(id string) (admin, error) {
	if id != "" {
		// Safeguard id for possible future use.
		return admin{}, common.ErrBadId
	}
	return admin{a.h}, nil
}

type admin struct {
	h *wsHandler
}

// Login implements the Login method on the Admin facade.
func (a admin) Login(req jujuparams.LoginRequest) (jujuparams.LoginResultV1, error) {
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

	modelTag := ""
	controllerTag := ""
	if a.h.modelUUID != "" {
		if err := a.h.jem.CheckCanRead(a.h.model); err != nil {
			return jujuparams.LoginResultV1{}, errgo.Mask(err, errgo.Is(params.ErrUnauthorized))
		}

		// If the UUID is for a model send a redirect error.
		if a.h.model.Id != a.h.controller.Id {
			return jujuparams.LoginResultV1{}, &jujuparams.Error{
				Code:    jujuparams.CodeRedirect,
				Message: "redirect required",
			}
		}

		modelTag = names.NewModelTag(a.h.model.UUID).String()
		controllerTag = names.NewModelTag(a.h.controller.UUID).String()
	}

	return jujuparams.LoginResultV1{
		// TODO(mhilton) Add user info
		ModelTag:      modelTag,
		ControllerTag: controllerTag,
		Facades: []jujuparams.FacadeVersions{{
			Name:     "Cloud",
			Versions: []int{1},
		}},
		ServerVersion: "2.0.0",
	}, nil
}

// RedirectInfo implements the RedirectInfo method on the Admin facade.
func (a admin) RedirectInfo() (jujuparams.RedirectInfoResult, error) {
	if a.h.jem.Auth.Username == "" {
		return jujuparams.RedirectInfoResult{}, params.ErrUnauthorized
	}
	if a.h.modelUUID == "" {
		return jujuparams.RedirectInfoResult{}, errgo.New("not redirected")
	}
	if err := a.h.jem.CheckCanRead(a.h.model); err != nil {
		return jujuparams.RedirectInfoResult{}, errgo.Mask(err, errgo.Is(params.ErrUnauthorized))
	}
	if a.h.model.Id == a.h.controller.Id {
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

type cloudRoot struct {
	h *wsHandler
}

func (c cloudRoot) Cloud(id string) (cloud, error) {
	if id != "" {
		// Safeguard id for possible future use.
		return cloud{}, common.ErrBadId
	}
	return cloud{c.h}, nil
}

// cloud implements the Cloud facade.
type cloud struct {
	h *wsHandler
}

func (c cloud) Cloud() (jujuparams.Cloud, error) {
	conn, err := c.h.jem.OpenAPI(c.h.model.Path)
	if err != nil {
		return jujuparams.Cloud{}, errgo.Mask(err)
	}
	defer conn.Close()
	var resp jujuparams.Cloud
	if err := conn.APICall("Cloud", 1, "", "Cloud", nil, &resp); err != nil {
		return jujuparams.Cloud{}, errgo.Mask(err, errgo.Any)
	}
	// TODO (mhilton) manipulate the result as required.
	return resp, nil
}

func (c cloud) Credentials(entities jujuparams.Entities) (jujuparams.CloudCredentialsResults, error) {
	results := make([]jujuparams.CloudCredentialsResult, len(entities.Entities))
	for i, ent := range entities.Entities {
		owner, err := names.ParseUserTag(ent.Tag)
		if err != nil {
			err = errgo.WithCausef(err, params.ErrBadRequest, "")
			results[i] = jujuparams.CloudCredentialsResult{
				Error: mapError(err).(*jujuparams.Error),
			}
			continue
		}
		creds, err := c.credentials(owner.Id())
		if err != nil {
			results[i] = jujuparams.CloudCredentialsResult{
				Error: mapError(err).(*jujuparams.Error),
			}
			continue
		}
		cloudCreds := make(map[string]jujuparams.CloudCredential, len(creds))
		for _, c := range creds {
			cloudCreds[string(c.Path.Name)] = jujuparams.CloudCredential{
				AuthType:   c.Type,
				Attributes: c.Attributes,
			}
		}
		results[i] = jujuparams.CloudCredentialsResult{
			Credentials: cloudCreds,
		}
	}
	return jujuparams.CloudCredentialsResults{
		Results: results,
	}, nil
}

func (c cloud) credentials(owner string) ([]mongodoc.Credential, error) {
	it := c.h.jem.CanReadIter(c.h.jem.DB.Credentials().Find(bson.D{{"path.user", owner}}).Iter())
	defer it.Close()
	var creds []mongodoc.Credential
	var cred mongodoc.Credential
	for it.Next(&cred) {
		creds = append(creds, cred)
	}
	return creds, it.Close()
}

func (c cloud) UpdateCredentials(args jujuparams.UsersCloudCredentials) (jujuparams.ErrorResults, error) {
	results := make([]jujuparams.ErrorResult, len(args.Users))
	for i, ucc := range args.Users {
		username, creds, err := c.parseCredentials(ucc)
		if err != nil {
			results[i] = jujuparams.ErrorResult{
				Error: mapError(err).(*jujuparams.Error),
			}
			continue
		}
		if err := c.h.jem.CheckACL([]string{username}); err != nil {
			results[i] = jujuparams.ErrorResult{
				Error: mapError(err).(*jujuparams.Error),
			}
			continue
		}
		if err := c.updateCredentials(creds); err != nil {
			results[i] = jujuparams.ErrorResult{
				Error: mapError(err).(*jujuparams.Error),
			}
		}
	}
	return jujuparams.ErrorResults{
		Results: results,
	}, nil
}

func (c cloud) parseCredentials(ucc jujuparams.UserCloudCredentials) (string, []mongodoc.Credential, error) {
	userTag, err := names.ParseUserTag(ucc.UserTag)
	if err != nil {
		return "", nil, errgo.WithCausef(err, params.ErrBadRequest, "")
	}
	var user params.User
	if err := user.UnmarshalText([]byte(userTag.Id())); err != nil {
		return "", nil, errgo.WithCausef(err, params.ErrBadRequest, "")
	}
	creds := make([]mongodoc.Credential, 0, len(ucc.Credentials))
	for name, cred := range ucc.Credentials {
		var n params.Name
		err := n.UnmarshalText([]byte(name))
		if err != nil {
			return "", nil, errgo.WithCausef(err, params.ErrBadRequest, "")
		}
		creds = append(creds, mongodoc.Credential{
			Path:         params.EntityPath{user, n},
			ProviderType: c.h.controller.ProviderType,
			Type:         cred.AuthType,
			Attributes:   cred.Attributes,
		})
	}
	return string(user), creds, nil
}

func (c cloud) updateCredentials(creds []mongodoc.Credential) error {
	for _, cred := range creds {
		err := c.h.jem.AddCredential(&cred)
		if err != nil {
			return errgo.Mask(err)
		}
	}
	return nil
}
