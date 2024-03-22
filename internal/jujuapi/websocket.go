// Copyright 2016 Canonical Ltd.

package jujuapi

import (
	"context"
	"database/sql"
	"net/http"
	"regexp"
	"time"

	"github.com/gorilla/websocket"
	"github.com/juju/juju/rpc"
	"github.com/juju/juju/rpc/jsoncodec"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/zaputil"
	"github.com/juju/zaputil/zapctx"
	"go.uber.org/zap"

	"github.com/canonical/jimm/internal/auth"
	"github.com/canonical/jimm/internal/dbmodel"
	"github.com/canonical/jimm/internal/errors"
	"github.com/canonical/jimm/internal/jimm"
	"github.com/canonical/jimm/internal/jimmhttp"
	jimmRPC "github.com/canonical/jimm/internal/rpc"
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

// GetAuthenticationService returns JIMM's oauth authentication service.
func (s *apiServer) GetAuthenticationService() jimm.OAuthAuthenticator {
	return s.jimm.OAuthAuthenticator
}

// ServeWS implements jimmhttp.WSServer.
func (s *apiServer) ServeWS(ctx context.Context, conn *websocket.Conn) {
	identityId := auth.SessionIdentityFromContext(ctx)
	controllerRoot := newControllerRoot(s.jimm, s.params, identityId)
	s.cleanup = controllerRoot.cleanup
	Dblogger := controllerRoot.newAuditLogger()
	serveRoot(context.Background(), controllerRoot, Dblogger, conn)
}

// Kill implements the rpc.Killer interface.
func (s *apiServer) Kill() {
	if s.cleanup != nil {
		s.cleanup()
	}
}

// serveRoot serves an RPC root object on a websocket connection.
func serveRoot(ctx context.Context, root root, logger jimm.DbAuditLogger, wsConn *websocket.Conn) {
	ctx = zapctx.WithFields(ctx, zap.Bool("websocket", true))

	// Note that although NewConn accepts a `RecorderFactory` input, the call to conn.ServeRoot
	// also accepts a `RecorderFactory` and will override anything set during the call to NewConn.
	conn := rpc.NewConn(
		jsoncodec.NewWebsocket(wsConn),
		nil,
	)
	rpcRecorderFactory := func() rpc.Recorder {
		return jimm.NewRecorder(logger)
	}
	conn.ServeRoot(root, rpcRecorderFactory, func(err error) error {
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

var extractPathInfo = regexp.MustCompile(`^\/?model\/(?P<modeluuid>\w{8}-\w{4}-\w{4}-\w{4}-\w{12})\/(?P<finalPath>.*)$`)
var modelIndex = mustGetSubexpIndex(extractPathInfo, "modeluuid")
var finalPathIndex = mustGetSubexpIndex(extractPathInfo, "finalPath")

func mustGetSubexpIndex(regex *regexp.Regexp, name string) int {
	index := regex.SubexpIndex(name)
	if index == -1 {
		panic("failed to find subexp index")
	}
	return index
}

// modelInfoFromPath takes a path to a model endpoint and returns the uuid
// and final path element. I.e. /model/<uuid>/api return <uuid>,api,err
// Basic validation of the uuid takes place.
func modelInfoFromPath(path string) (uuid string, finalPath string, err error) {
	matches := extractPathInfo.FindStringSubmatch(path)
	if len(matches) != 3 {
		return "", "", errors.E("invalid path")
	}
	return matches[modelIndex], matches[finalPathIndex], nil
}

// GetAuthenticationService returns JIMM's oauth authentication service.
func (s modelProxyServer) GetAuthenticationService() jimm.OAuthAuthenticator {
	return s.jimm.OAuthAuthenticator
}

// ServeWS implements jimmhttp.WSServer.
func (s modelProxyServer) ServeWS(ctx context.Context, clientConn *websocket.Conn) {
	jwtGenerator := jimm.NewJWTGenerator(&s.jimm.Database, s.jimm, s.jimm.JWTService)
	connectionFunc := controllerConnectionFunc(s, &jwtGenerator)
	zapctx.Debug(ctx, "Starting proxier")
	auditLogger := s.jimm.AddAuditLogEntry
	proxyHelpers := jimmRPC.ProxyHelpers{
		ConnClient:              clientConn,
		TokenGen:                &jwtGenerator,
		ConnectController:       connectionFunc,
		AuditLog:                auditLogger,
		JIMM:                    s.jimm,
		AuthenticatedIdentityID: auth.SessionIdentityFromContext(ctx),
	}
	jimmRPC.ProxySockets(ctx, proxyHelpers)
}

// controllerConnectionFunc returns a function that will be used to
// connect to a controller when a client makes a request.
func controllerConnectionFunc(s modelProxyServer, jwtGenerator *jimm.JWTGenerator) func(context.Context) (jimmRPC.WebsocketConnection, string, error) {
	return func(ctx context.Context) (jimmRPC.WebsocketConnection, string, error) {
		const op = errors.Op("proxy.controllerConnectionFunc")
		path := jimmhttp.PathElementFromContext(ctx, "path")
		zapctx.Debug(ctx, "grabbing model info from path", zap.String("path", path))
		uuid, finalPath, err := modelInfoFromPath(path)
		if err != nil {
			zapctx.Error(ctx, "error parsing path", zap.Error(err))
			return nil, "", errors.E(op, err)
		}
		m := dbmodel.Model{
			UUID: sql.NullString{
				String: uuid,
				Valid:  uuid != "",
			},
		}
		if err := s.jimm.Database.GetModel(context.Background(), &m); err != nil {
			zapctx.Error(ctx, "failed to find model", zap.String("uuid", uuid), zap.Error(err))
			return nil, "", errors.E(err, errors.CodeNotFound)
		}
		jwtGenerator.SetTags(m.ResourceTag(), m.Controller.ResourceTag())
		mt := m.ResourceTag()
		zapctx.Debug(ctx, "Dialing Controller", zap.String("path", path))
		controllerConn, err := jimmRPC.Dial(ctx, &m.Controller, mt, finalPath)
		if err != nil {
			zapctx.Error(ctx, "cannot dial controller", zap.String("controller", m.Controller.Name), zap.Error(err))
			return nil, "", err
		}
		fullModelName := m.Controller.Name + "/" + m.Name
		return controllerConn, fullModelName, nil
	}
}

// Use a 64k frame size for the websockets while we need to deal
// with x/net/websocket connections that don't deal with receiving
// fragmented messages.
const websocketFrameSize = 65536

var websocketUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
	// In order to deal with the remote side not handling message
	// fragmentation, we default to largeish frames.
	ReadBufferSize:  websocketFrameSize,
	WriteBufferSize: websocketFrameSize,
}
