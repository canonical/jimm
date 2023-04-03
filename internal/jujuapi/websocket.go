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
	if path == "" {
		return "", "", errors.E("Empty path")
	}
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
	jwtGenerator := jimm.NewJwtGenerator(s.jimm)
	connectionFunc := controllerConnectionFunc(s, &jwtGenerator)
	zapctx.Debug(ctx, "Starting proxier")
	jimmRPC.ProxySockets(ctx, clientConn, &jwtGenerator, connectionFunc)
}

// controllerConnectionFunc returns a function that will be used to
// connect to a controller when a client makes a request.
func controllerConnectionFunc(s modelProxyServer, jwtGenerator *jimm.JwtGenerator) func(context.Context) (*websocket.Conn, error) {
	connectToControllerFunc := func(ctx context.Context) (*websocket.Conn, error) {
		const op = errors.Op("proxy.controllerConnectionFunc")
		path := jimmhttp.PathElementFromContext(ctx, "path")
		uuid, finalPath, err := modelInfoFromPath(path)
		if err != nil {
			return nil, errors.E(op, err)
		}
		m := dbmodel.Model{
			UUID: sql.NullString{
				String: uuid,
				Valid:  uuid != "",
			},
		}
		if err := s.jimm.Database.GetModel(context.Background(), &m); err != nil {
			zapctx.Error(ctx, "failed to find model", zap.String("uuid", uuid), zap.Error(err))
			return nil, errors.E(err, errors.CodeNotFound)
		}
		jwtGenerator.SetTags(m.ResourceTag(), m.Controller.ResourceTag())
		mt := names.NewModelTag(uuid)
		zapctx.Debug(ctx, "Dialing Controller", zap.String("path", path))
		controllerConn, err := jujuclient.ProxyDial(ctx, &m.Controller, mt, finalPath)
		if err != nil {
			zapctx.Error(ctx, "cannot dial controller", zap.String("controller", m.Controller.Name), zap.Error(err))
			return nil, err
		}
		return controllerConn, nil
	}
	return connectToControllerFunc
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
