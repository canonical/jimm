// Copyright 2024 Canonical.

package jimmhttp

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"github.com/juju/zaputil/zapctx"
	"go.uber.org/zap"

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
	if h.Server == nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	ctx, authErr := h.Server.Authenticate(ctx, w, req)
	if authErr != nil {
		zapctx.Error(ctx, "authentication error", zap.Error(authErr))
		w.WriteHeader(http.StatusUnauthorized)
		_, err := w.Write([]byte(authErr.Error()))
		if err != nil {
			zapctx.Error(ctx, "failed to write authentication error", zap.Error(err))
		}
		return
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

	h.Server.ServeWS(ctx, conn)
}

func writeInternalServerErrorClosure(ctx context.Context, conn *websocket.Conn, err any) {
	data := websocket.FormatCloseMessage(websocket.CloseInternalServerErr, fmt.Sprintf("%v", err))
	if err := conn.WriteControl(websocket.CloseMessage, data, time.Time{}); err != nil {
		zapctx.Error(ctx, "cannot write close message", zap.Error(err))
	}
}

// A WSServer is a websocket server.
//
// ServeWS should handle all messaging on the websocket connection and
// return once the connection is no longer needed. The server will close
// the websocket connection, but not send any control messages.
type WSServer interface {
	ServeWS(context.Context, *websocket.Conn)

	Authenticate(ctx context.Context, w http.ResponseWriter, req *http.Request) (context.Context, error)
}
