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

	var root rpc.MethodFinder
	root = adminRoot{admin{h}}
	err := h.resolveUUID()
	if err != nil {
		root = &errRoot{err}
	}
	h.conn.ServeFinder(root, mapError)
	h.conn.Start()
	select {
	case <-h.conn.Dead():
	}
	h.conn.Close()
}

func (h *wsHandler) resolveUUID() error {
	var err error
	h.model, err = h.jem.ModelFromUUID(h.modelUUID)
	if err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrNotFound))
	}
	h.controller, err = h.jem.Controller(h.model.Controller)
	return errgo.Mask(err)
}

type admin struct {
	handler *wsHandler
}

func (a admin) Admin(id string) (admin, error) {
	if id != "" {
		// Safeguard id for possible future use.
		return admin{}, common.ErrBadId
	}
	return a, nil
}

// Login implements the Login method on the Admin facade.
func (a admin) Login(req jujuparams.LoginRequest) (jujuparams.LoginResultV1, error) {
	// JAAS only supports macaroon login, ignore all the other fields.
	attr, err := a.handler.jem.Bakery.CheckAny(req.Macaroons, nil, checkers.TimeBefore)
	if err != nil {
		if verr, ok := err.(*bakery.VerificationError); ok {
			m, err := a.handler.jem.NewMacaroon()
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
	a.handler.jem.Auth.Username = attr["username"]
	if err := a.handler.jem.CheckCanRead(a.handler.model); err != nil {
		return jujuparams.LoginResultV1{}, errgo.Mask(err, errgo.Is(params.ErrUnauthorized))
	}

	// Login successful
	a.handler.jem.Auth = jem.Authorization{attr["username"]}

	// If the UUID is for a model send a redirect error.
	if a.handler.model.Id != a.handler.controller.Id {
		return jujuparams.LoginResultV1{}, &jujuparams.Error{
			Code:    jujuparams.CodeRedirect,
			Message: "redirect required",
		}
	}

	// TODO (mhilton) serve some new methods
	return jujuparams.LoginResultV1{
		ModelTag:      names.NewModelTag(a.handler.model.UUID).String(),
		ControllerTag: names.NewModelTag(a.handler.controller.UUID).String(),
		ServerVersion: "2.0.0",
	}, nil
}

// RedirectInfo implements the RedirectInfo method on the Admin facade.
func (a admin) RedirectInfo() (jujuparams.RedirectInfoResult, error) {
	if a.handler.jem.Auth.Username == "" {
		return jujuparams.RedirectInfoResult{}, params.ErrUnauthorized
	}
	if err := a.handler.jem.CheckCanRead(a.handler.model); err != nil {
		return jujuparams.RedirectInfoResult{}, errgo.Mask(err, errgo.Is(params.ErrUnauthorized))
	}
	if a.handler.model.Id == a.handler.controller.Id {
		return jujuparams.RedirectInfoResult{}, errgo.New("not redirected")
	}
	nhps, err := network.ParseHostPorts(a.handler.controller.HostPorts...)
	if err != nil {
		return jujuparams.RedirectInfoResult{}, errgo.Mask(err)
	}
	hps := jujuparams.FromNetworkHostPorts(nhps)
	return jujuparams.RedirectInfoResult{
		Servers: [][]jujuparams.HostPort{hps},
		CACert:  a.handler.controller.CACert,
	}, nil
}

// adminRoot is a rpc.MethodFinder that implements the admin interface.
type adminRoot struct {
	admin
}

// FindMethod implements rpc.MethodFinder.
func (r adminRoot) FindMethod(rootName string, version int, methodName string) (rpcreflect.MethodCaller, error) {
	if rootName != "Admin" {
		return nil, &rpcreflect.CallNotImplementedError{
			RootMethod: rootName,
			Version:    version,
		}
	}
	if version < 3 {
		return nil, &rpc.RequestError{
			Code:    jujuparams.CodeNotSupported,
			Message: "JAAS does not support login from old clients",
		}
	}
	return rpcreflect.ValueOf(reflect.ValueOf(r.admin)).FindMethod("Admin", 0, methodName)
}

// errRoot is a rpc.MethodFinder that always returns an error.
type errRoot struct {
	err error
}

// FindMethod implements rpc.MethodFinder, but will always return (nil, err)
func (r *errRoot) FindMethod(rootName string, version int, methodName string) (rpcreflect.MethodCaller, error) {
	return nil, r.err
}
