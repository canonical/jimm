package rpc

import (
	"context"

	"github.com/gorilla/websocket"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/zaputil/zapctx"
	"go.uber.org/zap"

	"github.com/CanonicalLtd/jimm/internal/errors"
)

func handleError(err error, client, controller *websocket.Conn) {
	// We haven't sent a close message yet, so try to send one.
	for _, c := range []*websocket.Conn{client, controller} {
		c.CloseHandler()(websocket.CloseProtocolError, err.Error())
	}
}

// ManInTheMiddle takes two websocket connections, the first between a client and JIMM
// and the second between JIMM and a controller and acts as a man-in-the-middle forwarding
// requests from the client verbatim to the controller.
//
// The default close handler will send a close message when the websocket is closed.
// But this means that the other websocket is still open. To account for this:
// - If we receive a close from the server we should send a close to the client.
// - If we receive a close from the client we should send a close to the server.
// - If we encounter an error in other scenarios, we should close both websockets.
// The underlying TCP connections should be closed by the calling function.
//
// Note that this function assumes half-duplex communication i.e. a client sends a requests and
// expects a reply from the server. For a true mitm implementation, separate routines must
// be implemented for client->server and server->client.
func ManInTheMiddle(ctx context.Context, connClient, connController *websocket.Conn) {
	// TODO(Kian): Check if closing websockets is implemented properly. According to https://www.rfc-editor.org/rfc/rfc6455
	// the party that wants to close the connection sends a close frame and the other party
	// must respond with a close frame at which point the original party can close the connection.
	for {
		msg := new(message)
		if err := connClient.ReadJSON(msg); err != nil {
			if closeErr, ok := err.(*websocket.CloseError); ok {
				// Client sent a close message
				connController.CloseHandler()(closeErr.Code, closeErr.Text)
				break
			}
			handleError(err, connClient, connController)
			break
		}
		if msg.RequestID == 0 {
			// Use a 0 request ID to indicate that the message
			// received was not a valid RPC message.
			handleError(errors.E("received invalid RPC message"), connClient, connController)
			break
		}
		if !msg.isRequest() {
			// we received a response from the client which is not
			// supported
			zapctx.Error(ctx, "received response", zap.Any("message", msg))
			connClient.WriteJSON(message{
				RequestID: msg.RequestID,
				Error:     "not supported",
				ErrorCode: jujuparams.CodeNotSupported,
			})
			continue
		}
		zapctx.Info(ctx, "forwarding request", zap.Any("message", msg))
		err := connController.WriteJSON(msg)
		if err != nil {
			zapctx.Error(ctx, "cannot forward request", zap.Error(err))
			handleError(err, connClient, connController)
			break
		}
		response := new(message)
		if err := connController.ReadJSON(response); err != nil {
			if closeErr, ok := err.(*websocket.CloseError); ok {
				// Controller sent a close message
				connClient.CloseHandler()(closeErr.Code, closeErr.Text)
				break
			}
			handleError(err, connClient, connController)
			break
		}
		zapctx.Info(ctx, "received controller response", zap.Any("message", response))
		connClient.WriteJSON(response)
		if err != nil {
			zapctx.Error(ctx, "cannot return response", zap.Error(err))
			handleError(err, connClient, connController)
			break
		}
	}
}
