package rpc

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/zaputil/zapctx"
	"go.uber.org/zap"

	"github.com/CanonicalLtd/jimm/internal/errors"
	"github.com/CanonicalLtd/jimm/internal/jimm"
)

// TODO(Kian): Remove this once Juju updates their side.
type loginRequest struct {
	params.LoginRequest
	Token string `json:"token"`
}

// writeLockConn provides a websocket connection that is safe for concurrent writes.
type writeLockConn struct {
	mu   sync.Mutex
	conn *websocket.Conn
}

func (c *writeLockConn) readJson(v interface{}) error {
	return c.conn.ReadJSON(v)
}

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

// clientProxy proxies messages from client->controller
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
	defer func() {
		if p.controller != nil {
			p.controller.conn.Close()
		}
	}()
	for {
		msg := new(message)
		if err := p.client.readJson(msg); err != nil {
			zapctx.Error(ctx, "clientProxy error reading from src", zap.Error(err))
			return err
		}
		err := p.makeControllerConnection(ctx)
		if err != nil {
			zapctx.Error(ctx, "error connecting to controller", zap.Error(err))
			p.client.sendError(err, msg)
			return err
		}
		if msg.Type == "Admin" && msg.Request == "Login" {
			if err := addJWT(ctx, true, msg, nil, p.tokenGen); err != nil {
				p.client.sendError(err, msg)
				continue
			}
		}
		if msg.RequestID == 0 {
			err := errors.E(op, "Invalid request ID 0")
			p.client.sendError(err, msg)
			continue
		}
		p.msgs.addMessage(msg)
		if err := p.controller.writeJson(msg); err != nil {
			zapctx.Error(ctx, "clientProxy error writing to dst", zap.Error(err))
			p.client.sendError(err, msg)
			p.msgs.removeMessage(msg)
			continue
		}
	}
}

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
		msg := new(message)
		if err := p.controller.readJson(msg); err != nil {
			zapctx.Error(ctx, "controllerProxy error reading from src", zap.Error(err))
			return err
		}
		if msg.ErrorCode == "access required" {
			if err := p.redoLogin(ctx, msg); err != nil {
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
			modifyControllerResponse(msg)
			p.msgs.removeMessage(msg)
		}
		if err := p.client.writeJson(msg); err != nil {
			zapctx.Error(ctx, "controllerProxy error writing to dst", zap.Error(err))
			return err
		}
	}
}

func (p *controllerProxy) redoLogin(ctx context.Context, resp *message) error {
	const op = errors.Op("rpc.redoLogin")
	var loginMsg *message
	if msg, ok := p.msgs.messages[0]; ok {
		loginMsg = msg
	}
	if loginMsg == nil {
		return errors.E(op, errors.CodeUnauthorized, "Haven't received login yet")
	}
	err := addJWT(ctx, false, loginMsg, resp.ErrorInfo, p.tokenGen)
	if err != nil {
		return err
	}
	zapctx.Info(ctx, "Performing new login", zap.Any("message", loginMsg))
	if err := p.controller.writeJson(loginMsg); err != nil {
		zapctx.Error(ctx, "cannot send new login", zap.Error(err))
		return err
	}
	return nil
}

// addJWT adds a JWT token to the the provided message.
func addJWT(ctx context.Context, initialLogin bool, msg *message, permissions map[string]interface{}, tokenGen jimm.TokenGenerator) error {
	const op = errors.Op("rpc.addJWT")
	// First we unmarshal the existing LoginRequest.
	if msg == nil {
		return errors.E(op, "nil messsage")
	}
	var lr params.LoginRequest
	if err := json.Unmarshal(msg.Params, &lr); err != nil {
		return err
	}
	jwt, err := tokenGen.MakeToken(ctx, initialLogin, &lr, permissions)
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
		clProxy.wg.Done()
		errChan <- clProxy.start(ctx)
	}()
	var err error
	select {
	// No cleanup is needed on error, when the client closes the connection
	// all go routines will proceed to error and exit.
	case err = <-errChan:
	case <-ctx.Done():
		err = errors.E(op, "Context cancelled")
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
