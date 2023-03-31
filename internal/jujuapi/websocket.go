// Copyright 2016 Canonical Ltd.

package jujuapi

import (
	"context"
	"database/sql"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/juju/juju/rpc"
	"github.com/juju/juju/rpc/jsoncodec"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v4"
	"github.com/juju/zaputil"
	"github.com/juju/zaputil/zapctx"
	"go.uber.org/zap"

	"github.com/CanonicalLtd/jimm/internal/dbmodel"
	"github.com/CanonicalLtd/jimm/internal/errors"
	"github.com/CanonicalLtd/jimm/internal/jimm"
	"github.com/CanonicalLtd/jimm/internal/jimmhttp"
	"github.com/CanonicalLtd/jimm/internal/jujuclient"
	jimmRPC "github.com/CanonicalLtd/jimm/internal/rpc"
	"github.com/CanonicalLtd/jimm/internal/servermon"
)

const (
	requestTimeout        = 1 * time.Minute
	maxRequestConcurrency = 10
	pingTimeout           = 90 * time.Second
)

// A root is an rpc.Root enhanced so that it can notify on ping requests.
type root interface {
	rpc.Root
	setPingF(func())
}

// An apiServer is a jimmhttp.WSServer that serves the controller API.
type apiServer struct {
	jimm    *jimm.JIMM
	cleanup func()
	params  Params
}

// ServeWS implements jimmhttp.WSServer.
func (s apiServer) ServeWS(_ context.Context, conn *websocket.Conn) {
	controllerRoot := newControllerRoot(s.jimm, s.params)
	s.cleanup = controllerRoot.cleanup
	serveRoot(context.Background(), controllerRoot, conn)
}

// Kill implements the rpc.Killer interface.
func (s *apiServer) Kill() {
	if s.cleanup != nil {
		s.cleanup()
	}
}

// serveRoot serves an RPC root object on a websocket connection.
func serveRoot(ctx context.Context, root root, wsConn *websocket.Conn) {
	ctx = zapctx.WithFields(ctx, zap.Bool("websocket", true))

	conn := rpc.NewConn(
		jsoncodec.NewWebsocket(wsConn),
		func() rpc.Recorder {
			return recorder{
				start: time.Now(),
			}
		},
	)
	conn.ServeRoot(root, nil, func(err error) error {
		return mapError(err)
	})
	defer conn.Close()
	t := time.AfterFunc(pingTimeout, func() {
		zapctx.Info(ctx, "ping timeout, closing connection")
		conn.Close()
	})
	defer t.Stop()
	root.setPingF(func() { t.Reset(pingTimeout) })
	conn.Start(ctx)
	<-conn.Dead()
}

// mapError maps JIMM errors to errors suitable for use with the juju API.
func mapError(err error) *jujuparams.Error {
	if err == nil {
		return nil
	}
	// TODO the error mapper should really accept a context from the RPC package.
	zapctx.Debug(context.TODO(), "rpc error", zaputil.Error(err))

	return &jujuparams.Error{
		Message: err.Error(),
		Code:    string(errors.ErrorCode(err)),
	}
}

// A modelProxyServer serves the /commands and /api server for a model by
// proxying all requests through to the controller.
type modelProxyServer struct {
	jimm *jimm.JIMM
}

func modelInfoFromPath(path string) (uuid string, finalPath string, err error) {
	if path[0] == '/' {
		path = path[1:]
	}
	parts := strings.Split(path, "/")
	if len(parts) != 3 {
		return "", "", errors.E("Invalid path")
	}
	prefix := parts[0]
	uuid = parts[1]
	finalPath = parts[2]
	if prefix != "model" {
		return "", "", errors.E("Invalid path prefix")
	}
	err = nil
	return
}

// ServeWS implements jimmhttp.WSServer.
func (s modelProxyServer) ServeWS(ctx context.Context, clientConn *websocket.Conn) {
	sendClientError := func(err error) {
		msg := jujuparams.CLICommandStatus{
			Done:  true,
			Error: mapError(err),
		}
		// jujuparams.RequestError
		// type myError struct {
		// 	Error string `json:"error"`
		// }
		// msg := myError{
		// 	Error: "My Test Error",
		// }
		zapctx.Error(ctx, "Error was", zap.Any("Error", err))
		zapctx.Error(ctx, "Sending error to client", zap.Any("Message", msg))
		if err := clientConn.WriteJSON(mapError(err)); err != nil {
			zapctx.Error(ctx, "cannot send commands response", zap.Error(err))
		}
	}
	zapctx.Debug(context.Background(), "Dugtrio")
	path := jimmhttp.PathElementFromContext(ctx, "path")
	uuid, finalPath, err := modelInfoFromPath(path)
	if err != nil {
		sendClientError(err)
		return
	}
	zapctx.Debug(ctx, "Socket info", zap.String("uuid", uuid), zap.String("finalPath", finalPath))
	m := dbmodel.Model{
		UUID: sql.NullString{
			String: uuid,
			Valid:  uuid != "",
		},
	}
	if err := s.jimm.Database.GetModel(context.Background(), &m); err != nil {
		zapctx.Debug(ctx, "Model doesn't exist")
		sendClientError(err)
		return
	}
	mt := names.NewModelTag(uuid)
	zapctx.Debug(ctx, "Calling ProxyDial")
	controllerConn, err := jujuclient.ProxyDial(ctx, &m.Controller, mt, finalPath)
	if err != nil {
		zapctx.Error(ctx, "cannot dial controller", zap.String("controller", m.Controller.Name), zap.Error(err))
		sendClientError(err)
		return
	}
	defer controllerConn.Close()
	jwtGenerator := jimm.NewJwtAuthorizer(s.jimm, &m)
	zapctx.Error(ctx, "Starting Proxy Sockets")
	err = jimmRPC.ProxySockets(ctx, clientConn, controllerConn, &jwtGenerator)
	zapctx.Error(ctx, "After Proxy Sockets")
	// sendClientError(err)
}

// Use a 64k frame size for the websockets while we need to deal
// with x/net/websocket connections that don't deal with recieving
// fragmented messages.
const websocketFrameSize = 65536

var websocketUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
	// In order to deal with the remote side not handling message
	// fragmentation, we default to largeish frames.
	ReadBufferSize:  websocketFrameSize,
	WriteBufferSize: websocketFrameSize,
}

// recorder implements an rpc.Recorder.
type recorder struct {
	start time.Time
}

// HandleRequest implements rpc.Recorder.
func (recorder) HandleRequest(*rpc.Header, interface{}) error {
	return nil
}

// HandleReply implements rpc.Recorder.
func (o recorder) HandleReply(r rpc.Request, _ *rpc.Header, _ interface{}) error {
	d := time.Since(o.start)
	servermon.WebsocketRequestDuration.WithLabelValues(r.Type, r.Action).Observe(float64(d) / float64(time.Second))
	return nil
}
