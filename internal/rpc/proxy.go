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

// TokenGenerator generates a JWT token.
type TokenGenerator interface {
	MakeToken(ctx context.Context, req *params.LoginRequest, permissionMap map[string]interface{}) ([]byte, error)
}

type proxyInfo struct {
	connController *websocket.Conn
	connClient     *websocket.Conn
	tokenGen       TokenGenerator
	loginMsg       *message
}

// ProxySockets takes two websocket connections, the first between a client and JIMM
// and the second between JIMM and a controller and acts as a man-in-the-middle forwarding
// requests from the client verbatim to the controller.
//
// Closing the websockets should be handled by the calling function.
//
// Note that this function assumes half-duplex communication i.e. a client sends a request and
// expects a reply from the server as is done by Juju.
func ProxySockets(ctx context.Context, connClient, connController *websocket.Conn, tokenGen TokenGenerator) error {
	const op = errors.Op("rpc.ProxySockets")
	errChannel := make(chan error, 1)
	proxyInfo := proxyInfo{connController: connController, connClient: connClient, tokenGen: tokenGen}
	go func() {
		errChannel <- proxyInfo.proxy(ctx)
	}()
	var err error
	select {
	case err = <-errChannel:
	case <-ctx.Done():
		err = errors.E(op, "Context cancelled")
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

func (p proxyInfo) proxy(ctx context.Context) error {
	for {
		msg := new(message)
		if err := p.connClient.ReadJSON(msg); err != nil {
			zapctx.Error(ctx, "error reading from client", zap.Error(err))
			return err
		}
		// Add JWT if the user is sending a login request.
		if msg.Type == "Admin" && msg.Request == "Login" {
			if err := p.addJWT(ctx, true, msg, nil); err != nil {
				return err
			}
			// Store the login request for later.
			p.loginMsg = msg
		}

		// Loop the request to the controller in cases where we get an permission denied error.
	login:
		response := new(message)
		zapctx.Info(ctx, "forwarding request", zap.Any("message", msg))
		if err := p.connController.WriteJSON(msg); err != nil {
			zapctx.Error(ctx, "cannot forward request", zap.Error(err))
			return err
		}

		if err := p.connController.ReadJSON(response); err != nil {
			zapctx.Error(ctx, "error reading from controller", zap.Error(err))
			return err
		}
		// TODO(Kian): Check for juju.errors CodeAccessRequired
		if response.ErrorCode == "PermissionAssertionRequireError" {
			if err := p.redoLogin(ctx, response); err != nil {
				return err
			} else {
				goto login
			}
		}
		modifyControllerResponse(response)
		zapctx.Info(ctx, "received controller response", zap.Any("message", response))
		if err := p.connClient.WriteJSON(response); err != nil {
			zapctx.Error(ctx, "cannot return response", zap.Error(err))
			return err
		}
	}
}

func (p proxyInfo) redoLogin(ctx context.Context, resp *message) error {
	err := p.addJWT(ctx, false, p.loginMsg, resp.ErrorInfo)
	if err != nil {
		return err
	}
	zapctx.Info(ctx, "Performing new login", zap.Any("message", p.loginMsg))
	if err := p.connController.WriteJSON(p.loginMsg); err != nil {
		zapctx.Error(ctx, "cannot send new login", zap.Error(err))
		return err
	}
	if err := p.connController.ReadJSON(p.loginMsg); err != nil {
		zapctx.Error(ctx, "error login response from controller", zap.Error(err))
		return err
	}
	return nil
}

// addJWT adds a JWT token to the the provided message.
func (p proxyInfo) addJWT(ctx context.Context, performLogin bool, msg *message, permissions map[string]interface{}) error {
	const op = errors.Op("rpc.addJWT")
	// First we unmarshal the existing LoginRequest.
	if msg == nil {
		return errors.E(op, "nil messsage")
	}
	var lr params.LoginRequest
	if err := json.Unmarshal(msg.Params, &lr); err != nil {
		return err
	}
	var loginMsg *params.LoginRequest
	if performLogin {
		loginMsg = &lr
	} else {
		loginMsg = nil
	}
	jwt, err := p.tokenGen.MakeToken(ctx, loginMsg, permissions)
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

func modifyControllerResponse(msg *message) error {
	var response map[string]interface{}
	err := json.Unmarshal(msg.Response, &response)
	if err != nil {
		return err
	}
	// Delete servers block so that juju client's don't get redirected.
	if _, ok := response["servers"]; ok {
		delete(response, "servers")
	}
	newResp, err := json.Marshal(response)
	if err != nil {
		return err
	}
	msg.Response = newResp
	return nil
}
