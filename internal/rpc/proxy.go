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

type GetTokenFunc func(req *params.LoginRequest, errMap map[string]interface{}) ([]byte, error)

// ProxySockets takes two websocket connections, the first between a client and JIMM
// and the second between JIMM and a controller and acts as a man-in-the-middle forwarding
// requests from the client verbatim to the controller.
//
// Closing the websockets should be handled by the calling function.
//
// Note that this function assumes half-duplex communication i.e. a client sends a request and
// expects a reply from the server as is done by Juju.
func ProxySockets(ctx context.Context, connClient, connController *websocket.Conn, f GetTokenFunc) error {
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

func proxy(ctx context.Context, connClient, connController *websocket.Conn, f GetTokenFunc) error {
	var loginMsg *message
	var skipClientRead bool
	// readCallback is called after a message is read whether from the client or the controller.
	// it return a boolean indicating whether to continue and an error.
	readCallback := func(msg *message) (bool, error) {
		// Handle login requests from client
		if msg.Type == "Admin" && msg.Request == "Login" {
			if err := addJWT(msg, nil, f); err != nil {
				return false, err
			}
			loginMsg = msg
			return false, nil
		}
		// Handle permission denied from controller - repeat login.
		if msg.ErrorCode == "PermissionAssertionRequireError" {
			if err := addJWT(loginMsg, msg.ErrorInfo, f); err != nil {
				return false, err
			}
			skipClientRead = true
			return true, nil
		}
		return false, nil
	}

	for {
		var msg *message
		if skipClientRead {
			msg = loginMsg
			skipClientRead = false
		} else {
			msg = new(message)
			if err := connClient.ReadJSON(msg); err != nil {
				zapctx.Error(ctx, "error reading from client", zap.Error(err))
				return err
			}
			if _, err := readCallback(msg); err != nil {
				return err
			}
		}

		response := new(message)
		zapctx.Info(ctx, "forwarding request", zap.Any("message", msg))
		if err := connController.WriteJSON(msg); err != nil {
			zapctx.Error(ctx, "cannot forward request", zap.Error(err))
			return err
		}

		if err := connController.ReadJSON(response); err != nil {
			zapctx.Error(ctx, "error reading from controller", zap.Error(err))
			return err
		}
		if proxyContinue, err := readCallback(response); err != nil {
			return err
		} else if proxyContinue {
			continue
		}

		zapctx.Info(ctx, "received controller response", zap.Any("message", response))
		if err := connClient.WriteJSON(response); err != nil {
			zapctx.Error(ctx, "cannot return response", zap.Error(err))
			return err
		}
	}
}

func addJWT(msg *message, permissions map[string]interface{}, f GetTokenFunc) error {
	// First we unmarshal the existing LoginRequest.
	if msg == nil {
		return errors.E("nil messsage")
	}
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
