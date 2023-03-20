package rpc

import (
	"context"
	"encoding/base64"
	"encoding/json"

	"github.com/gorilla/websocket"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/zaputil/zapctx"
	"go.uber.org/zap"

	"github.com/CanonicalLtd/jimm/internal/errors"
)

type AuthFunc func(req *params.LoginRequest, errMap map[string]interface{}) ([]byte, error)

// ProxySockets takes two websocket connections, the first between a client and JIMM
// and the second between JIMM and a controller and acts as a man-in-the-middle forwarding
// requests from the client verbatim to the controller.
//
// Closing the websockets should be handled by the calling function.
//
// Note that this function assumes half-duplex communication i.e. a client sends a request and
// expects a reply from the server as is done by Juju.
func ProxySockets(ctx context.Context, connClient, connController *websocket.Conn, f AuthFunc) error {
	errChannel := make(chan error, 1)
	go func() {
		errChannel <- proxy(ctx, connClient, connController, f)
	}()
	var err error
	select {
	case err = <-errChannel:
	case <-ctx.Done():
		err = errors.E("Context cancelled")
		connClient.Close()
		connController.Close()
	}
	return err
}

// TODO(Kian): Remove this once Juju updates their side.
type loginRequest struct {
	params.LoginRequest

	Token string `json:"token"`
}

func proxy(ctx context.Context, connClient, connController *websocket.Conn, f AuthFunc) error {
	var loginMsg *message
	for {
		msg := new(message)
		if err := connClient.ReadJSON(msg); err != nil {
			zapctx.Error(ctx, "error reading from client", zap.Error(err))
			return err
		}
		// Add JWT to message
		if msg.Type == "Admin" && msg.Request == "Login" {
			if err := addJWT(msg, nil, f); err != nil {
				return err
			}
			loginMsg = msg
		}

		// Loop the request to the controller in case we get an permissionDenied error.
		response := new(message)
		for {
			zapctx.Info(ctx, "forwarding request", zap.Any("message", msg))
			if err := connController.WriteJSON(msg); err != nil {
				zapctx.Error(ctx, "cannot forward request", zap.Error(err))
				return err
			}

			if err := connController.ReadJSON(response); err != nil {
				zapctx.Error(ctx, "error reading from controller", zap.Error(err))
				return err
			}
			if response.ErrorCode == "PermissionAssertionRequireError" {
				if err := redoLogin(ctx, loginMsg, response, connClient, connController, f); err != nil {
					return err
				}
			} else {
				break
			}
		}

		zapctx.Info(ctx, "received controller response", zap.Any("message", response))
		if err := connClient.WriteJSON(response); err != nil {
			zapctx.Error(ctx, "cannot return response", zap.Error(err))
			return err
		}
	}
}

func redoLogin(ctx context.Context, loginMsg *message, resp *message, connClient, connController *websocket.Conn, f AuthFunc) error {
	err := addJWT(loginMsg, resp.ErrorInfo, f)
	if err != nil {
		return err
	}
	zapctx.Info(ctx, "Performing new login", zap.Any("message", loginMsg))
	if err := connController.WriteJSON(loginMsg); err != nil {
		zapctx.Error(ctx, "cannot send new login", zap.Error(err))
		return err
	}
	if err := connClient.ReadJSON(loginMsg); err != nil {
		zapctx.Error(ctx, "error login response from controller", zap.Error(err))
		return err
	}
	// TODO(Kian): Add logic to determine if login was successful.
	// This response won't be forwarded back to the client.
	return nil
}

func addJWT(msg *message, permissions map[string]interface{}, f AuthFunc) error {
	// First we unmarshal the existing LoginRequest.
	var lr params.LoginRequest
	if err := json.Unmarshal(msg.Params, &lr); err != nil {
		return err
	}
	jwt, err := f(&lr, permissions)
	if err != nil {
		return err
	}
	jwtString := base64.StdEncoding.EncodeToString(jwt)
	// Add the JWT as base64 encoded string.
	loginRequest := loginRequest{
		LoginRequest: lr,
		Token:        jwtString,
	}
	// Marshal it again to JSON.
	data, err := json.Marshal(loginRequest)
	if err != nil {
		return err
	}
	// And add it to the message.
	msg.Params = data
	return nil
}
