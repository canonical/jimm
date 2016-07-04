// Copyright 2016 Canonical Ltd.

package jujuapi

import (
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/observer"
	"github.com/juju/juju/rpc"
	"github.com/juju/juju/rpc/jsoncodec"
	"github.com/juju/juju/rpc/rpcreflect"
	"golang.org/x/net/websocket"

	"github.com/CanonicalLtd/jem/internal/jem"
)

// newWSServer creates a new websocket server suitible for handling the api for modelUUID.
func newWSServer(jem *jem.JEM, modelUUID string) websocket.Server {
	hnd := wsHandler{
		jem:       jem,
		modelUUID: modelUUID,
	}
	return websocket.Server{
		Handler: hnd.handle,
	}
}

// wsHandler is a handler for a particular websocket connection.
type wsHandler struct {
	jem       *jem.JEM
	modelUUID string
}

// handle handles the connection.
func (h *wsHandler) handle(wsConn *websocket.Conn) {
	codec := jsoncodec.NewWebsocket(wsConn)
	conn := rpc.NewConn(codec, observer.None())

	// TODO(mhilton) serve something useful on this connection.
	err := common.UnknownModelError(h.modelUUID)
	conn.ServeFinder(&errRoot{err}, serverError)
	conn.Start()
	select {
	case <-conn.Dead():
	}
	conn.Close()
}

func serverError(err error) error {
	if err := common.ServerError(err); err != nil {
		return err
	}
	return nil
}

// errRoot implements the API that a client first sees
// when connecting to the API. It exposes the same API as initialRoot, except
// it returns the requested error when the client makes any request.
type errRoot struct {
	err error
}

// FindMethod conforms to the same API as initialRoot, but we'll always return (nil, err)
func (r *errRoot) FindMethod(rootName string, version int, methodName string) (rpcreflect.MethodCaller, error) {
	return nil, r.err
}
