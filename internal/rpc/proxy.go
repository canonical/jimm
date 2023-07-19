package rpc

import (
	"context"
	"encoding/base64"
	"encoding/json"
	stderrors "errors"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/names/v4"
	"github.com/juju/zaputil/zapctx"
	"go.uber.org/zap"

	"github.com/canonical/jimm/internal/auth"
	"github.com/canonical/jimm/internal/dbmodel"
	"github.com/canonical/jimm/internal/errors"
)

// TokenGenerator authenticates a user and generates a JWT token.
type TokenGenerator interface {
	// MakeToken authorizes a user if initialLogin is set to true using the information in req.
	// It then checks that a user has all the default permissions rquired and then checks for
	// permissions as required by permissionMap. It then returns a JWT token.
	MakeToken(ctx context.Context, initialLogin bool, req *params.LoginRequest, permissionMap map[string]interface{}) ([]byte, error)
	// SetTags sets the desired model and controller tags that this TokenGenerator is valid for.
	SetTags(mt names.ModelTag, ct names.ControllerTag)
	// GetUser returns the authenticated user.
	GetUser() names.UserTag
}

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

type modelProxy struct {
	src            *writeLockConn
	dst            *writeLockConn
	msgs           *inflightMsgs
	auditLog       func(*dbmodel.AuditLogEntry)
	tokenGen       TokenGenerator
	modelName      string
	conversationId string
}

func (p *modelProxy) sendError(socket *writeLockConn, req *message, err error) {
	if req == nil {
		// If there was no message to error on, just return.
		return
	}
	msg := createErrResponse(err, req)
	if msg != nil {
		socket.writeJson(msg)
	}
	// An error message is a response back to the client.
	p.auditLogMessage(msg, true)
}

func (p *modelProxy) auditLogMessage(msg *message, isResponse bool) error {
	ale := dbmodel.AuditLogEntry{
		Time:           time.Now().UTC().Round(time.Millisecond),
		MessageId:      msg.RequestID,
		UserTag:        p.tokenGen.GetUser().String(),
		Model:          p.modelName,
		ConversationId: p.conversationId,
		FacadeName:     msg.Type,
		FacadeMethod:   msg.Request,
		FacadeVersion:  msg.Version,
		IsResponse:     isResponse,
		ObjectId:       msg.ID,
	}

	// For responses extract errors. For requests extract params.
	if isResponse {
		// Extract errors from bulk and non-bulk calls.
		var allErrors params.ErrorResults
		if msg.Response != nil {
			err := json.Unmarshal(msg.Response, &allErrors)
			if err != nil {
				zapctx.Error(context.Background(), "failed to unmarshal message response", zap.Error(err), zap.Any("message", msg))
				return errors.E(err, "failed to unmarshal message response")
			}
		}
		singleError := params.ErrorResult{Error: &params.Error{Message: msg.Error, Code: msg.ErrorCode, Info: msg.ErrorInfo}}
		allErrors.Results = append(allErrors.Results, singleError)
		jsonErr, err := json.Marshal(allErrors)
		if err != nil {
			return errors.E(err, "failed to marshal all errors")
		}
		ale.Errors = jsonErr
	} else {
		jsonBody, err := json.Marshal(msg.Params)
		if err != nil {
			zapctx.Error(context.Background(), "failed to marshal body", zap.Error(err))
			return err
		}
		ale.Params = jsonBody
	}
	p.auditLog(&ale)
	return nil
}

// clientProxy proxies messages from client->controller.
type clientProxy struct {
	modelProxy
	wg                   sync.WaitGroup
	errChan              chan error
	createControllerConn func(context.Context) (*websocket.Conn, string, error)
	// mu synchronises changes to closed and modelproxy.dst, dst is is only created
	// at some unspecified point in the future after a client request.
	mu     sync.Mutex
	closed bool
}

// start begins the client->controller proxier.
func (p *clientProxy) start(ctx context.Context) error {
	const op = errors.Op("rpc.clientProxy.start")
	const initialLogin = true
	defer func() {
		if p.dst != nil {
			p.dst.conn.Close()
		}
	}()
	for {
		zapctx.Debug(ctx, "Reading on client connection")
		msg := new(message)
		if err := p.src.readJson(&msg); err != nil {
			zapctx.Error(ctx, "clientProxy error reading from src", zap.Error(err))
			return err
		}
		zapctx.Debug(ctx, "Read message from client", zap.Any("message", msg))
		err := p.makeControllerConnection(ctx)
		if err != nil {
			zapctx.Error(ctx, "error connecting to controller", zap.Error(err))
			p.sendError(p.src, msg, err)
			return err
		}
		p.auditLogMessage(msg, false)
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
						p.sendError(p.src, msg, err)
						return err
					}
					p.src.sendMessage(res, msg)
					continue
				}
				p.sendError(p.src, msg, err)
				continue
			}
		}
		if msg.RequestID == 0 {
			zapctx.Error(ctx, "Invalid request ID 0")
			err := errors.E(op, "Invalid request ID 0")
			p.sendError(p.src, msg, err)
			continue
		}
		p.msgs.addMessage(msg)
		zapctx.Debug(ctx, "Writing to controller")
		if err := p.dst.writeJson(msg); err != nil {
			zapctx.Error(ctx, "clientProxy error writing to dst", zap.Error(err))
			p.sendError(p.src, msg, err)
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
	if p.dst != nil {
		return nil
	}
	// Checking closed ensures we don't have a race condition with a cancelled context.
	if p.closed {
		err := errors.E(op, "Client connection closed while starting controller connection")
		return err
	}
	conn, modelName, err := p.createControllerConn(ctx)
	if err != nil {
		return err
	}
	p.modelName = modelName
	p.dst = &writeLockConn{conn: conn}
	controllerToClient := controllerProxy{
		modelProxy: modelProxy{
			src:            p.dst,
			dst:            p.src,
			msgs:           p.msgs,
			auditLog:       p.auditLog,
			tokenGen:       p.tokenGen,
			modelName:      p.modelName,
			conversationId: p.conversationId,
		},
	}
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
	modelProxy
}

// start implements the controller->client proxier.
func (p *controllerProxy) start(ctx context.Context) error {
	for {
		zapctx.Debug(ctx, "Reading on controller connection")
		msg := new(message)
		if err := p.src.readJson(msg); err != nil {
			zapctx.Error(ctx, "controllerProxy error reading from src", zap.Error(err))
			// Error reading on the socket implies it is closed, simply return.
			return err
		}
		zapctx.Debug(ctx, "Received message from controller", zap.Any("Message", msg))
		permissionsRequired, err := p.checkPermissionsRequired(ctx, msg)
		if err != nil {
			zapctx.Error(ctx, "failed to determine if more permissions required", zap.Error(err))
			p.handleError(msg, err)
			continue
		}
		if permissionsRequired != nil {
			zapctx.Error(ctx, "Access Required error")
			if err := p.redoLogin(ctx, permissionsRequired); err != nil {
				zapctx.Error(ctx, "Failed to redo login", zap.Error(err))
				p.handleError(msg, err)
				continue
			}
			// Write back to the controller.
			msg := p.msgs.getMessage(msg.RequestID)
			if msg != nil {
				p.src.writeJson(msg)
			}
			continue
		} else {
			if err := modifyControllerResponse(msg); err != nil {
				zapctx.Error(ctx, "Failed to modify message", zap.Error(err))
				p.handleError(msg, err)
				// An error when modifying the message is a show stopper.
				return err
			}
		}
		p.msgs.removeMessage(msg)
		p.auditLogMessage(msg, true)
		zapctx.Debug(ctx, "Writing modified message to client", zap.Any("Message", msg))
		if err := p.dst.writeJson(msg); err != nil {
			zapctx.Error(ctx, "controllerProxy error writing to dst", zap.Error(err))
			return err
		}
	}
}

func (p *controllerProxy) handleError(msg *message, err error) {
	p.sendError(p.dst, msg, err)
	p.msgs.removeMessage(msg)
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
	if err := p.src.writeJson(loginMsg); err != nil {
		return err
	}
	return nil
}

// addJWT adds a JWT token to the the provided message.
// If initialLogin is set the user will be authenticated.
func addJWT(ctx context.Context, initialLogin bool, msg *message, permissions map[string]interface{}, tokenGen TokenGenerator) error {
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

// ProxyHelpers contains all the necessary helpers for proxying a Juju client
// connection to a model.
type ProxyHelpers struct {
	ConnClient        *websocket.Conn
	TokenGen          TokenGenerator
	ConnectController func(context.Context) (*websocket.Conn, string, error)
	AuditLog          func(*dbmodel.AuditLogEntry)
}

// ProxySockets will proxy requests from a client connection through to a controller
// tokenGen is used to authenticate the user and generate JWT token.
// connectController provides the function to return a connection to the desired controller endpoint.
func ProxySockets(ctx context.Context, helpers ProxyHelpers) error {
	const op = errors.Op("rpc.ProxySockets")
	if helpers.ConnectController == nil {
		zapctx.Error(ctx, "Missing controller connect function")
		return errors.E(op, "Missing controller connect function")
	}
	if helpers.AuditLog == nil {
		zapctx.Error(ctx, "Missing audit log function")
		return errors.E(op, "Missing audit log function")
	}
	errChan := make(chan error, 2)
	msgInFlight := inflightMsgs{messages: make(map[uint64]*message)}
	client := writeLockConn{conn: helpers.ConnClient}
	// Note that the clProxy start method will create the connection to the desired controller only
	// after the first message has been received so that any errors can be properly sent back to the client.
	clProxy := clientProxy{
		modelProxy: modelProxy{
			src:      &client,
			msgs:     &msgInFlight,
			tokenGen: helpers.TokenGen,
			auditLog: helpers.AuditLog,
		},
		errChan:              errChan,
		createControllerConn: helpers.ConnectController,
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
		helpers.ConnClient.Close()
		clProxy.mu.Lock()
		clProxy.closed = true
		// TODO(Kian): Test removing close on dst below. The client connection should do it.
		if clProxy.dst != nil {
			clProxy.dst.conn.Close()
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
	// Delete servers block so that juju clients don't get redirected.
	delete(response, "servers")
	newResp, err := json.Marshal(response)
	if err != nil {
		return err
	}
	msg.Response = newResp
	return nil
}
