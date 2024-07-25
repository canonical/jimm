package rpc

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"sync"
	"time"

	"github.com/juju/juju/rpc/params"
	"github.com/juju/names/v5"
	"github.com/juju/zaputil/zapctx"
	"go.uber.org/zap"
	"golang.org/x/oauth2"

	apiparams "github.com/canonical/jimm/api/params"
	"github.com/canonical/jimm/internal/dbmodel"
	"github.com/canonical/jimm/internal/errors"
	"github.com/canonical/jimm/internal/jimm"
	"github.com/canonical/jimm/internal/jimm/credentials"
	"github.com/canonical/jimm/internal/openfga"
	"github.com/canonical/jimm/internal/servermon"
	"github.com/canonical/jimm/internal/utils"
	jimmnames "github.com/canonical/jimm/pkg/names"
)

const (
	accessRequiredErrorCode = "access required"
)

// TokenGenerator authenticates a user and generates a JWT token.
type TokenGenerator interface {
	// MakeLoginToken returns a JWT containing claims about user's access
	// to the controller, model (if applicable) and all clouds that the
	// controller knows about.
	MakeLoginToken(ctx context.Context, user *openfga.User) ([]byte, error)
	// MakeToken assumes MakeLoginToken has already been called and checks the permissions
	// specified in the permissionMap. If the logged in user has all those permissions
	// a JWT will be returned with assertions confirming all those permissions.
	MakeToken(ctx context.Context, permissionMap map[string]interface{}) ([]byte, error)
	// SetTags sets the desired model and controller tags that this TokenGenerator is valid for.
	SetTags(mt names.ModelTag, ct names.ControllerTag)
	// GetUser returns the authenticated user.
	GetUser() names.UserTag
}

// WebsocketConnection represents the websocket connection interface used by the proxy.
type WebsocketConnection interface {
	ReadJSON(v interface{}) error
	WriteJSON(v interface{}) error
	Close() error
}

// WebsocketConnectionWithMetadata holds the websocket connection and metadata about the
// established connection.
type WebsocketConnectionWithMetadata struct {
	Conn           WebsocketConnection
	ControllerUUID string
	ModelName      string
}

// JIMM represents the JIMM interface used by the proxy.
type JIMM interface {
	GetOpenFGAUserAndAuthorise(ctx context.Context, email string) (*openfga.User, error)
	OAuthAuthenticationService() jimm.OAuthAuthenticator
	GetCredentialStore() credentials.CredentialStore
}

// ProxyHelpers contains all the necessary helpers for proxying a Juju client
// connection to a model.
type ProxyHelpers struct {
	ConnClient              WebsocketConnection
	TokenGen                TokenGenerator
	ConnectController       func(context.Context) (WebsocketConnectionWithMetadata, error)
	AuditLog                func(*dbmodel.AuditLogEntry)
	JIMM                    JIMM
	AuthenticatedIdentityID string
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
			src:                     &client,
			msgs:                    &msgInFlight,
			tokenGen:                helpers.TokenGen,
			auditLog:                helpers.AuditLog,
			conversationId:          utils.NewConversationID(),
			jimm:                    helpers.JIMM,
			authenticatedIdentityID: helpers.AuthenticatedIdentityID,
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

// writeLockConn provides a websocket connection that is safe for concurrent writes.
type writeLockConn struct {
	mu   sync.Mutex
	conn WebsocketConnection
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

func (c *writeLockConn) sendMessage(responseObject any, request *message) {
	msg := new(message)
	msg.RequestID = request.RequestID
	msg.Response = request.Response
	if responseObject != nil {
		responseData, err := json.Marshal(responseObject)
		if err != nil {
			errorMsg := createErrResponse(err, request)
			c.writeJson(errorMsg)
		}
		msg.Response = responseData
	}
	c.writeJson(msg)
}

// inflightMsgs holds only request messages that are
// still pending a response from a Juju controller.
type inflightMsgs struct {
	controllerUUID string

	mu           sync.Mutex
	loginMessage *message
	messages     map[uint64]*message
}

func (msgs *inflightMsgs) addLoginMessage(msg *message) {
	msgs.mu.Lock()
	defer msgs.mu.Unlock()

	msgs.loginMessage = msg
}

func (msgs *inflightMsgs) getLoginMessage() *message {
	msgs.mu.Lock()
	defer msgs.mu.Unlock()

	return msgs.loginMessage
}

func (msgs *inflightMsgs) addMessage(msg *message) {
	msgs.mu.Lock()
	defer msgs.mu.Unlock()

	msg.start = time.Now()
	msgs.messages[msg.RequestID] = msg
}

// removeMessage deletes the request message that corresponds
// to the responses message ID.
func (msgs *inflightMsgs) removeMessage(msgID uint64) {
	msgs.mu.Lock()
	req, ok := msgs.messages[msgID]
	if ok {
		delete(msgs.messages, msgID)
	}
	msgs.mu.Unlock()

	if ok {
		servermon.JujuCallDurationHistogram.WithLabelValues(
			req.Type,
			req.Request,
			msgs.controllerUUID,
		).Observe(time.Since(req.start).Seconds())
	}
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
	src                     *writeLockConn
	dst                     *writeLockConn
	msgs                    *inflightMsgs
	auditLog                func(*dbmodel.AuditLogEntry)
	tokenGen                TokenGenerator
	jimm                    JIMM
	modelName               string
	conversationId          string
	authenticatedIdentityID string

	deviceOAuthResponse *oauth2.DeviceAuthResponse
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
	servermon.JujuCallErrorCount.WithLabelValues(req.Type, req.Request, p.msgs.controllerUUID)
	p.auditLogMessage(msg, true)
}

func (p *modelProxy) auditLogMessage(msg *message, isResponse bool) error {
	ale := dbmodel.AuditLogEntry{
		Time:           time.Now().UTC().Round(time.Millisecond),
		MessageId:      msg.RequestID,
		IdentityTag:    p.tokenGen.GetUser().String(),
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
	createControllerConn func(context.Context) (WebsocketConnectionWithMetadata, error)
	// mu synchronises changes to closed and modelproxy.dst, dst is is only created
	// at some unspecified point in the future after a client request.
	mu     sync.Mutex
	closed bool
}

// start begins the client->controller proxier.
func (p *clientProxy) start(ctx context.Context) error {
	const op = errors.Op("rpc.clientProxy.start")
	defer func() {
		if p.dst != nil {
			p.dst.conn.Close()
		}
	}()
	for {
		zapctx.Debug(ctx, "Reading on client connection")
		msg := new(message)
		if err := p.src.readJson(&msg); err != nil {
			// Error reading on the socket implies it is closed, simply return.
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
		if msg.Type == "Admin" {
			zapctx.Debug(ctx, "handling an Admin facade call")
			toClient, toController, err := p.handleAdminFacade(ctx, msg)
			if err != nil {
				p.sendError(p.src, msg, err)
				continue
			}
			// If there is a response for the client, send it to the client and continue.
			// If there is a message for the controller instead, use the normal path.
			// We can't send the client a response from JIMM and send a message to the controller.
			if toClient != nil {
				p.src.sendMessage(nil, toClient)
				continue
			} else if toController != nil {
				msg = toController
				p.msgs.addLoginMessage(toController)
			}
		}
		p.msgs.addMessage(msg)
		zapctx.Debug(ctx, "Writing to controller")
		if err := p.dst.writeJson(msg); err != nil {
			zapctx.Error(ctx, "clientProxy error writing to dst", zap.Error(err))
			p.sendError(p.src, msg, err)
			p.msgs.removeMessage(msg.RequestID)
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
	connWithMetadata, err := p.createControllerConn(ctx)
	if err != nil {
		return err
	}

	p.msgs.controllerUUID = connWithMetadata.ControllerUUID

	p.modelName = connWithMetadata.ModelName
	p.dst = &writeLockConn{conn: connWithMetadata.Conn}
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
			// Error reading on the socket implies it is closed, simply return.
			return err
		}
		zapctx.Debug(ctx, "Received message from controller", zap.Any("Message", msg))
		permissionsRequired, err := checkPermissionsRequired(ctx, msg)
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
		p.msgs.removeMessage(msg.RequestID)
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
	p.msgs.removeMessage(msg.RequestID)
}

// checkPermissionsRequired returns a nil map if no permissions are required.
func checkPermissionsRequired(ctx context.Context, msg *message) (map[string]any, error) {
	// Instantiate later because we won't always need the map.
	var permissionMap map[string]any

	// Check for errors that may be a result of a normal request.
	if msg.ErrorCode == accessRequiredErrorCode {
		permissionMap = msg.ErrorInfo
		return permissionMap, nil
	}

	// if the message response is empty, this is clearly not a permission
	// check required error and we return an empty map of required
	// permissions
	if msg.Response == nil || string(msg.Response) == "" {
		return permissionMap, nil
	}

	var er params.ErrorResults
	err := json.Unmarshal(msg.Response, &er)
	if err != nil {
		zapctx.Error(ctx, "failed to read response error")
		return permissionMap, nil
	}

	// Check for errors that may be a result of a bulk request.
	for _, e := range er.Results {
		zapctx.Debug(ctx, "received error", zap.Any("error", e))
		if e.Error != nil && e.Error.Code == accessRequiredErrorCode {
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
	return permissionMap, nil
}

func (p *controllerProxy) redoLogin(ctx context.Context, permissions map[string]any) error {
	const op = errors.Op("rpc.redoLogin")

	loginMsg := p.msgs.getLoginMessage()
	if loginMsg == nil {
		return errors.E(op, errors.CodeUnauthorized, "Haven't received login yet")
	}
	err := addJWT(ctx, loginMsg, permissions, p.tokenGen)
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
func addJWT(ctx context.Context, msg *message, permissions map[string]interface{}, tokenGen TokenGenerator) error {
	const op = errors.Op("rpc.addJWT")
	// First we unmarshal the existing LoginRequest.
	if msg == nil {
		return errors.E(op, "nil messsage")
	}
	var lr params.LoginRequest
	if err := json.Unmarshal(msg.Params, &lr); err != nil {
		return errors.E(op, err)
	}

	jwt, err := tokenGen.MakeToken(ctx, permissions)
	if err != nil {
		zapctx.Error(ctx, "failed to make token", zap.Error(err))
		return errors.E(op, err)
	}

	jwtString := base64.StdEncoding.EncodeToString(jwt)
	// Add the JWT as base64 encoded string.
	lr.Token = jwtString
	// Marshal it again to JSON.
	data, err := json.Marshal(lr)
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

// handleAdminFacade processes the admin facade call and returns:
// a message to be returned to the source
// a message to be sent to the destination
// an error
func (p *clientProxy) handleAdminFacade(ctx context.Context, msg *message) (clientResponse *message, controllerMessage *message, err error) {
	errorFnc := func(err error) (*message, *message, error) {
		return nil, nil, err
	}
	controllerLoginMessageFnc := func(data []byte) (*message, *message, error) {
		m := *msg
		m.Type = "Admin"
		m.Request = "Login"
		m.Version = 3
		m.Params = data
		return nil, &m, nil
	}
	switch msg.Request {
	case "LoginDevice":
		deviceResponse, err := jimm.LoginDevice(ctx, p.jimm.OAuthAuthenticationService())
		if err != nil {
			return errorFnc(err)
		}
		p.deviceOAuthResponse = deviceResponse

		data, err := json.Marshal(apiparams.LoginDeviceResponse{
			VerificationURI: deviceResponse.VerificationURI,
			UserCode:        deviceResponse.UserCode,
		})
		if err != nil {
			return errorFnc(err)
		}
		msg.Response = data
		return msg, nil, nil
	case "GetDeviceSessionToken":
		sessionToken, err := jimm.GetDeviceSessionToken(ctx, p.jimm.OAuthAuthenticationService(), p.jimm.GetCredentialStore(), p.deviceOAuthResponse)
		if err != nil {
			return errorFnc(err)
		}
		data, err := json.Marshal(apiparams.GetDeviceSessionTokenResponse{
			SessionToken: sessionToken,
		})
		if err != nil {
			return errorFnc(err)
		}
		msg.Response = data
		return msg, nil, nil
	case "LoginWithSessionToken":
		var request apiparams.LoginWithSessionTokenRequest
		err := json.Unmarshal(msg.Params, &request)
		if err != nil {
			return errorFnc(err)
		}

		// Verify the session token
		token, err := p.jimm.OAuthAuthenticationService().VerifySessionToken(request.SessionToken)
		if err != nil {
			return errorFnc(err)
		}
		email := token.Subject()

		user, err := p.jimm.GetOpenFGAUserAndAuthorise(ctx, email)
		if err != nil {
			return errorFnc(err)
		}

		jwt, err := p.tokenGen.MakeLoginToken(ctx, user)
		if err != nil {
			return errorFnc(err)
		}
		data, err := json.Marshal(params.LoginRequest{
			AuthTag: names.NewUserTag(email).String(),
			Token:   base64.StdEncoding.EncodeToString(jwt),
		})
		if err != nil {
			return errorFnc(err)
		}
		return controllerLoginMessageFnc(data)
	case "LoginWithClientCredentials":
		var request apiparams.LoginWithClientCredentialsRequest
		err := json.Unmarshal(msg.Params, &request)
		if err != nil {
			return errorFnc(err)
		}
		clientIdWithDomain, err := jimmnames.EnsureValidServiceAccountId(request.ClientID)
		if err != nil {
			return errorFnc(err)
		}
		err = p.jimm.OAuthAuthenticationService().VerifyClientCredentials(ctx, request.ClientID, request.ClientSecret)
		if err != nil {
			return errorFnc(err)
		}

		user, err := p.jimm.GetOpenFGAUserAndAuthorise(ctx, clientIdWithDomain)
		if err != nil {
			return errorFnc(err)
		}

		jwt, err := p.tokenGen.MakeLoginToken(ctx, user)
		if err != nil {
			return errorFnc(err)
		}
		data, err := json.Marshal(params.LoginRequest{
			AuthTag: names.NewUserTag(clientIdWithDomain).String(),
			Token:   base64.StdEncoding.EncodeToString(jwt),
		})
		if err != nil {
			return errorFnc(err)
		}
		return controllerLoginMessageFnc(data)
	case "LoginWithSessionCookie":
		if p.modelProxy.authenticatedIdentityID == "" {
			return errorFnc(errors.E(errors.CodeUnauthorized))
		}
		user, err := p.jimm.GetOpenFGAUserAndAuthorise(ctx, p.modelProxy.authenticatedIdentityID)
		if err != nil {
			return errorFnc(err)
		}

		jwt, err := p.tokenGen.MakeLoginToken(ctx, user)
		if err != nil {
			return errorFnc(err)
		}
		data, err := json.Marshal(params.LoginRequest{
			AuthTag: user.ResourceTag().String(),
			Token:   base64.StdEncoding.EncodeToString(jwt),
		})
		if err != nil {
			return errorFnc(err)
		}
		return controllerLoginMessageFnc(data)
	case "Login":
		return errorFnc(errors.E("JIMM does not support login from old clients", errors.CodeNotSupported))
	default:
		return nil, nil, nil
	}
}
