// Copyright 2024 Canonical.

package jujuapi

import (
	"context"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"github.com/juju/juju/rpc"
	"github.com/juju/juju/rpc/jsoncodec"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/zaputil"
	"github.com/juju/zaputil/zapctx"
	"go.uber.org/zap"

	"github.com/canonical/jimm/v3/internal/auth"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/jimm"
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

// Authenticate implements jimmhttp.Authenticate and handles browser authentication
// when a session cookie is present, ultimately placing the identity resolved from
// the cookie within the passed context.
//
// It updates the response header on authentication errors with a InternalServerError,
// and as such is safe to return from your handler upon error without updating
// the response statuses.
func (s *apiServer) Authenticate(ctx context.Context, w http.ResponseWriter, req *http.Request) (context.Context, error) {
	// We perform cookie authentication at the HTTP layer instead of WS
	// due to limitations of setting and retrieving cookies in the WS layer.
	//
	// If no cookie is present, we expect 1 of 3 scenarios:
	// 1. It's a device session token login.
	// 2. It's a client credential login.
	// 3. It's an "expired" cookie login, and as such no cookie
	//	  has been sent with the request. The handling of this is within
	//    LoginWithSessionCookie, in which, due to no identityId being present
	//    we know the cookie expired or a request with no cookie was made.
	_, err := req.Cookie(auth.SessionName)

	// Now we know a cookie is present, so let's try perform a cookie login / logic
	// as presumably a cookie of this name should only ever be present in the case
	// the browser performs a connection.
	if err == nil {
		ctx, err = s.jimm.OAuthAuthenticator.AuthenticateBrowserSession(ctx, w, req)
		if err != nil {
			zapctx.Error(ctx, "authenticate browser session failed", zap.Error(err))
			// Something went wrong when trying to perform the authentication
			// of the cookie.
			return ctx, err
		}
	}

	// If there's an error due to failure to find the cookie, just return the context
	// and move on presuming it's a device or client credentials login.
	return ctx, nil
}

// ServeWS implements jimmhttp.WSServer.
func (s *apiServer) ServeWS(ctx context.Context, conn *websocket.Conn) {
	identityId := auth.SessionIdentityFromContext(ctx)
	controllerRoot := newControllerRoot(s.jimm, s.params, identityId)
	s.cleanup = controllerRoot.cleanup
	Dblogger := controllerRoot.newAuditLogger()
	serveRoot(ctx, controllerRoot, Dblogger, conn)
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
