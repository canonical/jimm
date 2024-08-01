// Copyright 2021 Canonical Ltd.

package jimmhttp

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"github.com/juju/zaputil/zapctx"
	"go.uber.org/zap"

	"github.com/canonical/jimm/v3/internal/auth"
	"github.com/canonical/jimm/v3/internal/jimm"
	"github.com/canonical/jimm/v3/internal/servermon"
)

// A WSHandler is an http.Handler that upgrades the connection to a
// websocket and starts a Server with the upgraded connection.
type WSHandler struct {
	// Upgrader is the websocket.Upgrader to use to upgrade the
	// connection.
	Upgrader websocket.Upgrader

	// Server is the websocket server that will handle the websocket
	// connection.
	Server WSServer
}

// ServeHTTP implements http.Handler by upgrading the HTTP request to a
// websocket connection and running Server.ServeWS with the upgraded
// connection. ServeHTTP returns as soon as the websocket connection has
// been started.
func (h *WSHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()
	var authErr error

	if h.Server != nil && h.Server.GetAuthenticationService() != nil {
		ctx, authErr = handleBrowserAuthentication(
			ctx,
			h.Server.GetAuthenticationService(),
			w,
			req,
		)
	}

	ctx = context.WithValue(ctx, contextPathKey("path"), req.URL.EscapedPath())
	conn, err := h.Upgrader.Upgrade(w, req, nil)
	if err != nil {
		// If the upgrader returns an error it will have written an
		// error response, so there is no need to do so here.
		zapctx.Error(ctx, "cannot upgrade websocket", zap.Error(err))
		return
	}

	servermon.ConcurrentWebsocketConnections.Inc()
	defer conn.Close()
	defer servermon.ConcurrentWebsocketConnections.Dec()
	defer func() {
		if err := recover(); err != nil {
			zapctx.Error(ctx, "websocket panic", zap.Any("err", err), zap.Stack("stack"))
			writeInternalServerErrorClosure(ctx, conn, err)
		}
	}()

	if authErr != nil {
		zapctx.Error(ctx, "browser authentication error", zap.Any("err", authErr), zap.Stack("stack"))
		writeInternalServerErrorClosure(ctx, conn, authErr)
		return
	}

	if h.Server == nil {
		writeNormalClosure(ctx, conn)
		return
	}

	h.Server.ServeWS(ctx, conn)
}

func writeNormalClosure(ctx context.Context, conn *websocket.Conn) {
	data := websocket.FormatCloseMessage(websocket.CloseNormalClosure, "")
	if err := conn.WriteControl(websocket.CloseMessage, data, time.Time{}); err != nil {
		zapctx.Error(ctx, "cannot write close message", zap.Error(err))
	}
}

func writeInternalServerErrorClosure(ctx context.Context, conn *websocket.Conn, err any) {
	data := websocket.FormatCloseMessage(websocket.CloseInternalServerErr, fmt.Sprintf("%v", err))
	if err := conn.WriteControl(websocket.CloseMessage, data, time.Time{}); err != nil {
		zapctx.Error(ctx, "cannot write close message", zap.Error(err))
	}
}

// handleBrowserAuthentication handles browser authentication when a session cookie
// is present, ultimately placing the identity resolved from the cookie within the
// passed context.
//
// It updates the response header on authentication errors with a InternalServerError,
// and as such is safe to return from your handler upon error without updating
// the response statuses.
func handleBrowserAuthentication(ctx context.Context, authSvc jimm.OAuthAuthenticator, w http.ResponseWriter, req *http.Request) (context.Context, error) {
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
		ctx, err = authSvc.AuthenticateBrowserSession(
			ctx, w, req,
		)
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

// A WSServer is a websocket server.
//
// ServeWS should handle all messaging on the websocket connection and
// return once the connection is no longer needed. The server will close
// the websocket connection, but not send any control messages.
type WSServer interface {
	ServeWS(context.Context, *websocket.Conn)

	// GetAuthenticationService returns JIMM's authentication services.
	GetAuthenticationService() jimm.OAuthAuthenticator
}
