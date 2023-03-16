package rpc

import (
	"context"
	"fmt"

	"github.com/gorilla/websocket"
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
		fmt.Printf("Proxy reading\n")
		if err := connClient.ReadJSON(msg); err != nil {
			fmt.Printf("Error read client - %s\n", err.Error())
			return err
		}

		fmt.Printf("Proxy read from client: %+v\n", msg)
		if msg.RequestID == 0 {
			fmt.Printf("Error request id = 0")
			return errors.E("Received invalid RPC message")
		}

		// if !msg.isRequest() {
		// 	zapctx.Error(ctx, "received response", zap.Any("message", msg))
		// 	connClient.WriteJSON(message{
		// 		RequestID: msg.RequestID,
		// 		Error:     "not supported",
		// 		ErrorCode: jujuparams.CodeNotSupported,
		// 	})
		// 	continue
		// }
		fmt.Printf("Forwarding request to controller %+v\n", msg)

		zapctx.Info(ctx, "forwarding request", zap.Any("message", msg))
		msg.Type = "ProxyType"
		if err := connController.WriteJSON(msg); err != nil {
			fmt.Printf("Error from controller - %s\n", err.Error())
			zapctx.Error(ctx, "cannot forward request", zap.Error(err))
			return err
		}

		response := new(message)
		// TODO(Kian): If we receive a permissions error below we will need a new error code and the calling
		// function should recalculate permissions, re-do login and perform the request again.
		fmt.Printf("Proxy reading from controller\n")
		if err := connController.ReadJSON(response); err != nil {
			fmt.Printf("Error from controller - %s\n", err.Error())
			return err
		}
		fmt.Printf("Proxy read %+v from controller, writing to client.\n", response)

		zapctx.Info(ctx, "received controller response", zap.Any("message", response))
		if err := connClient.WriteJSON(response); err != nil {
			fmt.Printf("Error write client - %s\n", err.Error())
			zapctx.Error(ctx, "cannot return response", zap.Error(err))
			return err
		}
		fmt.Printf("Proxy wrote %+v back to the client.\n", response)
	}
}
