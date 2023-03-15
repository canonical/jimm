package rpc

import (
	"context"

	"github.com/gorilla/websocket"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/zaputil/zapctx"
	"go.uber.org/zap"

	"github.com/CanonicalLtd/jimm/internal/errors"
)

// ProxySockets takes two websocket connections, the first between a client and JIMM
// and the second between JIMM and a controller and acts as a man-in-the-middle forwarding
// requests from the client verbatim to the controller.
//
// Closing the websockets should be handled by the calling function.
//
// Note that this function assumes half-duplex communication i.e. a client sends a request and
// expects a reply from the server. For a true mitm implementation, separate routines must
// be implemented for client->server and server->client.
func ProxySockets(ctx context.Context, connClient, connController *websocket.Conn) error {
	for {
		msg := new(message)
		if err := connClient.ReadJSON(msg); err != nil {
			return err
		}
		if msg.RequestID == 0 {
			return errors.E("Received invalid RPC message")
		}
		if !msg.isRequest() {
			zapctx.Error(ctx, "received response", zap.Any("message", msg))
			connClient.WriteJSON(message{
				RequestID: msg.RequestID,
				Error:     "not supported",
				ErrorCode: jujuparams.CodeNotSupported,
			})
			continue
		}
		zapctx.Info(ctx, "forwarding request", zap.Any("message", msg))
		if err := connController.WriteJSON(msg); err != nil {
			zapctx.Error(ctx, "cannot forward request", zap.Error(err))
			return err
		}
		response := new(message)
		// TODO(Kian): If we receive a permissions error below we will need a new error code and the calling
		// function should recalculate permissions, re-do login and perform the request again.
		if err := connController.ReadJSON(response); err != nil {
			return err
		}
		zapctx.Info(ctx, "received controller response", zap.Any("message", response))
		if err := connClient.WriteJSON(response); err != nil {
			zapctx.Error(ctx, "cannot return response", zap.Error(err))
			return err
		}
	}
}
