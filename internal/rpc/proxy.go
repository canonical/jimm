package rpc

import (
	"context"
	"encoding/base64"
	"encoding/json"
	stderrors "errors"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/zaputil/zapctx"
	"go.uber.org/zap"

	"github.com/CanonicalLtd/jimm/internal/auth"
	"github.com/CanonicalLtd/jimm/internal/errors"
	"github.com/CanonicalLtd/jimm/internal/jimm"
)

// TODO(Kian): Remove this once we update our Juju library.
type loginRequest struct {
	params.LoginRequest
	Token string `json:"token"`
}

// writeLockConn provides a websocket connection that is safe for concurrent writes.
type writeLockConn struct {
	mu   sync.Mutex
	conn *websocket.Conn
}

// readJson allows for non-concurrent reads on the websocket.
func (c *writeLockConn) readJson(v interface{}) error {
	return c.conn.ReadJSON(v)
}

// writeJson allows for concurrent writes on the websocket.
func (c *writeLockConn) writeJson(v interface{}) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.conn.WriteJSON(v)
}

func (c *writeLockConn) sendError(err error, req *message) {
	if req == nil {
		// If there was no message to error on, just return.
		return
	}
	msg := createErrResponse(err, req)
	if msg != nil {
		c.writeJson(msg)
	}
}

func (c *writeLockConn) sendMessage(responseData json.RawMessage, request *message) {
	msg := new(message)
	msg.RequestID = request.RequestID
	msg.Response = responseData
	c.writeJson(msg)
}

type inflightMsgs struct {
	mu       sync.Mutex
	messages map[uint64]*message
}

func (msgs *inflightMsgs) addMessage(msg *message) {
	msgs.mu.Lock()
	defer msgs.mu.Unlock()
	// Putting the login request on ID 0 to persist it.
	if msg.Type == "Admin" && msg.Request == "Login" {
		msgs.messages[0] = msg
	} else {
		msgs.messages[msg.RequestID] = msg
	}
}

func (msgs *inflightMsgs) removeMessage(msg *message) {
	msgs.mu.Lock()
	defer msgs.mu.Unlock()
	delete(msgs.messages, msg.RequestID)
}

func (msgs *inflightMsgs) getMessage(key uint64) *message {
	msgs.mu.Lock()
	defer msgs.mu.Unlock()
	msg, ok := msgs.messages[key]
	if !ok {
		return nil
	}
	return msg
}

// clientProxy proxies messages from client->controller.
type clientProxy struct {
	// mu synchronises changes to closed and dst, dst is is only created
	// at some unspecified point in the future after a client request.
	mu         sync.Mutex
	client     *writeLockConn
	controller *writeLockConn
	closed     bool

	msgs                 *inflightMsgs
	tokenGen             jimm.TokenGenerator
	createControllerConn func(context.Context) (*websocket.Conn, error)
	wg                   sync.WaitGroup
	errChan              chan error
}

// start begins the client->controller proxier.
func (p *clientProxy) start(ctx context.Context) error {
	const op = errors.Op("rpc.clientProxy.start")
	const initialLogin = true
	defer func() {
		if p.controller != nil {
			p.controller.conn.Close()
		}
	}()
	for {
		zapctx.Debug(ctx, "Reading on client connection")
		msg := new(message)
		if err := p.client.readJson(&msg); err != nil {
			zapctx.Error(ctx, "clientProxy error reading from src", zap.Error(err))
			return err
		}
		zapctx.Debug(ctx, "Read message from client", zap.Any("message", msg))
		err := p.makeControllerConnection(ctx)
		if err != nil {
			zapctx.Error(ctx, "error connecting to controller", zap.Error(err))
			p.client.sendError(err, msg)
			return err
		}
		// All requests should be proxied as transparently as possible through to the controller
		// except for auth related requests like Login because JIMM is auth gateway.
		if msg.Type == "Admin" && msg.Request == "Login" {
			zapctx.Debug(ctx, "Login request found, adding JWT")
			if err := addJWT(ctx, initialLogin, msg, nil, p.tokenGen); err != nil {
				zapctx.Error(ctx, "Failed to add JWT", zap.Error(err))
				var aerr *auth.AuthenticationError
				if stderrors.As(err, &aerr) {
					res, err := json.Marshal(aerr.LoginResult)
					if err != nil {
						p.client.sendError(err, msg)
						return err
					}
					p.client.sendMessage(res, msg)
					continue
				}
				p.client.sendError(err, msg)
				continue
			}
		}
		if msg.RequestID == 0 {
			zapctx.Error(ctx, "Invalid request ID 0")
			err := errors.E(op, "Invalid request ID 0")
			p.client.sendError(err, msg)
			continue
		}
		p.msgs.addMessage(msg)
		zapctx.Debug(ctx, "Writing to controller")
		if err := p.controller.writeJson(msg); err != nil {
			zapctx.Error(ctx, "clientProxy error writing to dst", zap.Error(err))
			p.client.sendError(err, msg)
			p.msgs.removeMessage(msg)
			continue
		}
	}
}

// makeControllerConnection dials a controller and starts a go routine for
// proxying requests from the controller to the client.
func (p *clientProxy) makeControllerConnection(ctx context.Context) error {
	const op = errors.Op("rpc.makeControllerConnection")
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.controller != nil {
		return nil
	}
	// Checking closed ensures we don't have a race condition with a cancelled context.
	if p.closed {
		err := errors.E(op, "Client connection closed while starting controller connection")
		return err
	}
	conn, err := p.createControllerConn(ctx)
	if err != nil {
		return err
	}
	p.controller = &writeLockConn{conn: conn}
	controllerToClient := controllerProxy{controller: p.controller, client: p.client, msgs: p.msgs, tokenGen: p.tokenGen}
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		p.errChan <- controllerToClient.start(ctx)
	}()
	zapctx.Debug(ctx, "Successfully made controller connection")
	return nil
}

// controllerProxy proxies messages from controller->client with the caveat that
// it will retry client->controller messages that require further permissions.
type controllerProxy struct {
	controller *writeLockConn
	client     *writeLockConn
	// msgs tracks in flight messages
	msgs     *inflightMsgs
	tokenGen jimm.TokenGenerator
}

// start implements the controller->client proxier.
func (p *controllerProxy) start(ctx context.Context) error {
	for {
		zapctx.Debug(ctx, "Reading on controller connection")
		msg := new(message)
		if err := p.controller.readJson(msg); err != nil {
			zapctx.Error(ctx, "controllerProxy error reading from src", zap.Error(err))
			return err
		}
		zapctx.Debug(ctx, "Received message from controller", zap.Any("Message", msg))
		permissionsRequired, err := p.checkPermissionsRequired(ctx, msg)
		if err != nil {
			zapctx.Error(ctx, "failed to determine if more permissions required", zap.Error(err))
			p.client.sendError(err, msg)
			p.msgs.removeMessage(msg)
			continue
		}
		if permissionsRequired != nil {
			zapctx.Error(ctx, "Access Required error")
			if err := p.redoLogin(ctx, permissionsRequired); err != nil {
				zapctx.Error(ctx, "Failed to redo login", zap.Error(err))
				p.client.sendError(err, msg)
				p.msgs.removeMessage(msg)
				continue
			}
			// Write back to the controller.
			msg := p.msgs.getMessage(msg.RequestID)
			if msg != nil {
				p.controller.writeJson(msg)
			}
			continue
		} else {
			if err := modifyControllerResponse(msg); err != nil {
				zapctx.Error(ctx, "Failed to modify message", zap.Error(err))
				p.client.sendError(err, msg)
				return err
			}
			p.msgs.removeMessage(msg)
		}
		zapctx.Debug(ctx, "Writing modified message to client", zap.Any("Message", msg))
		if err := p.client.writeJson(msg); err != nil {
			zapctx.Error(ctx, "controllerProxy error writing to dst", zap.Error(err))
			return err
		}
	}
}

// checkPermissionsRequired returns a nil map if no permissions are required.
func (p *controllerProxy) checkPermissionsRequired(ctx context.Context, msg *message) (map[string]any, error) {
	var er params.ErrorResults
	err := json.Unmarshal(msg.Response, &er)
	if err != nil {
		zapctx.Error(ctx, "failed to read response error")
		return nil, errors.E(err, "failed to read response errors")
	}
	// Instantiate later because we won't always need the map.
	var permissionMap map[string]any
	// Check for errors that may be a result of a bulk request.
	for _, e := range er.Results {
		zapctx.Debug(ctx, "received error", zap.Any("error", e))
		if e.Error != nil && e.Error.Code == "access required" {
			for k, v := range e.Error.Info {
				accessLevel, ok := v.(string)
				if !ok {
					return nil, errors.E("unknown permission level")
				}
				if permissionMap == nil {
					permissionMap = make(map[string]any)
				}
				permissionMap[k] = accessLevel
			}
		}
	}
	// Check for errors that may be a result of a normal request.
	if msg.ErrorCode == "access required" {
		if permissionMap != nil {
			zapctx.Error(ctx, "detected access required error in two places")
		}
		permissionMap = msg.ErrorInfo
	}
	return permissionMap, nil
}

func (p *controllerProxy) redoLogin(ctx context.Context, permissions map[string]any) error {
	const op = errors.Op("rpc.redoLogin")
	const initialLogin = false
	var loginMsg *message
	if msg, ok := p.msgs.messages[0]; ok {
		loginMsg = msg
	}
	if loginMsg == nil {
		return errors.E(op, errors.CodeUnauthorized, "Haven't received login yet")
	}
	err := addJWT(ctx, initialLogin, loginMsg, permissions, p.tokenGen)
	if err != nil {
		return err
	}
	zapctx.Info(ctx, "Performing new login", zap.Any("message", loginMsg))
	if err := p.controller.writeJson(loginMsg); err != nil {
		return err
	}
	return nil
}

// addJWT adds a JWT token to the the provided message.
// If initialLogin is set the user will be authenticated.
func addJWT(ctx context.Context, initialLogin bool, msg *message, permissions map[string]interface{}, tokenGen jimm.TokenGenerator) error {
	const op = errors.Op("rpc.addJWT")
	// First we unmarshal the existing LoginRequest.
	if msg == nil {
		return errors.E(op, "nil messsage")
	}
	var lr params.LoginRequest
	if err := json.Unmarshal(msg.Params, &lr); err != nil {
		return errors.E(op, err)
	}
	jwt, err := tokenGen.MakeToken(ctx, initialLogin, &lr, permissions)
	if err != nil {
		zapctx.Error(ctx, "failed to make token", zap.Error(err))
		return errors.E(op, err)
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
		return errors.E(op, err)
	}
	// And add it to the message.
	msg.Params = data
	return nil
}

func createErrResponse(err error, req *message) *message {
	errMsg := new(message)
	errMsg.RequestID = req.RequestID
	errMsg.Error = err.Error()
	errMsg.ErrorCode = string(errors.ErrorCode(err))
	return errMsg
}

// ProxySockets will proxy requests from a client connection through to a controller
// tokenGen is used to authenticate the user and generate JWT token.
// connectController provides the function to return a connection to the desired controller endpoint.
func ProxySockets(ctx context.Context, connClient *websocket.Conn, tokenGen jimm.TokenGenerator, connectController func(context.Context) (*websocket.Conn, error)) error {
	const op = errors.Op("rpc.ProxySockets")
	errChan := make(chan error, 2)
	msgInFlight := inflightMsgs{messages: make(map[uint64]*message)}
	client := writeLockConn{conn: connClient}
	// Note that the clProxy start method will create the connection to the desired controller only
	// after the first message has been received so that any errors can be properly sent back to the client.
	clProxy := clientProxy{
		client:               &client,
		msgs:                 &msgInFlight,
		tokenGen:             tokenGen,
		errChan:              errChan,
		createControllerConn: connectController,
	}
	clProxy.wg.Add(1)
	go func() {
		defer clProxy.wg.Done()
		errChan <- clProxy.start(ctx)
	}()
	var err error
	select {
	// No cleanup is needed on error, when the client closes the connection
	// all go routines will proceed to error and exit.
	case err = <-errChan:
		zapctx.Debug(ctx, "Proxy error", zap.Error(err))
	case <-ctx.Done():
		err = errors.E(op, "Context cancelled")
		zapctx.Debug(ctx, "Context cancelled")
		connClient.Close()
		clProxy.mu.Lock()
		clProxy.closed = true
		if clProxy.controller != nil {
			clProxy.controller.conn.Close()
		}
		clProxy.mu.Unlock()
	}
	clProxy.wg.Wait()
	return err
}

func modifyControllerResponse(msg *message) error {
	var response map[string]interface{}
	err := json.Unmarshal(msg.Response, &response)
	if err != nil {
		return err
	}
	// Delete servers block so that juju client's don't get redirected.
	delete(response, "servers")
	newResp, err := json.Marshal(response)
	if err != nil {
		return err
	}
	msg.Response = newResp
	return nil
}
