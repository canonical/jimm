// Copyright 2021 Canonical Ltd.

package jimmhttp

import (
	"context"
	"net/http"

	"github.com/gorilla/websocket"
	"github.com/juju/zaputil/zapctx"
	"go.uber.org/zap"

	"github.com/CanonicalLtd/jimm/internal/servermon"
)

// A WSHandler is an http.Handler that upgrades the connection to a
// websocket and starts a Server with the upgraded connection.
type WSHandler struct {
	// Upgrader is the websocket.Upgrader to use to upgrade the
	// connection.
	Upgrader websocket.Upgrader

	// Server is the server that will be started by the handler.
	Server WSServer
}

// ServeHTTP implements http.Handler by upgrading the HTTP request to a
// websocket connection and running Server.ServeWS with the upgraded
// connection. ServeHTTP returns as soon as the websocket connection has
// been started.
func (h *WSHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()

	conn, err := h.Upgrader.Upgrade(w, req, nil)
	if err != nil {
		// If the upgrader returns an error it will have written an
		// error response, so there is no need to do so here.
		zapctx.Error(ctx, "cannot upgrade websocket", zap.Error(err))
		return
	}
	servermon.ConcurrentWebsocketConnections.Inc()
	go func() {
		defer conn.Close()
		defer servermon.ConcurrentWebsocketConnections.Dec()
		defer func() {
			if err := recover(); err != nil {
				zapctx.Error(ctx, "websocket panic", zap.Any("err", err), zap.Stack("stack"))
			}
		}()
		if h.Server != nil {
			h.Server.ServeWS(ctx, conn)
		}
	}()
}

// A WSServer is a websocket server.
//
// ServeWS should handle all messaging on the websocket connection and
// return once the connection is no longer needed. The server will close
// the websocket connection, but not send any control messages.
type WSServer interface {
	ServeWS(context.Context, *websocket.Conn)
}
