// Copyright 2025 Canonical.

// Package rpcproxy implements a proxy for Juju's RPC messages.
// The rpcproxy is used to proxy messages between jimm and model facades
// on Juju controllers while still acting as an authorisation and routing layer.
package rpcproxy

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v5"
	"github.com/juju/zaputil/zapctx"
	"go.uber.org/zap"
	"golang.org/x/oauth2"

	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/logger"
	"github.com/canonical/jimm/v3/internal/openfga"
	"github.com/canonical/jimm/v3/internal/servermon"
)

const (
	accessRequiredErrorCode = "access required"
)

// ControllerDetails holds information about the controller
// that is being proxied to.
type ControllerDetails struct {
	Addresses [][]jujuparams.HostPort
	CACert    string
}

// RedirectInfoGetter provides information about the controller
// that is being proxied to.T his information is useful
// when we need to redirect a client to that controller
// instead of proxying the request. This is the case during
// model migration when receiving requests from agents.
type RedirectInfoGetter interface {
	GetRedirectInfo(ctx context.Context) (ControllerDetails, error)
}

// TokenGenerator authenticates a user and generates a JWT token.
type TokenGenerator interface {
	// MakeLoginToken returns a JWT containing claims about user's access
	// to the controller, model (if applicable) and all clouds that the
	// controller knows about.
	MakeLoginToken(ctx context.Context, user *openfga.User) ([]byte, error)
	// MakeToken assumes MakeLoginToken has already been called and checks the permissions
	// specified in the permissionMap. If the logged in user has all those permissions
	// a JWT will be returned with assertions confirming all those permissions.
	MakeToken(ctx context.Context, permissionMap map[string]any) ([]byte, error)
	// SetTags sets the desired model and controller tags that this TokenGenerator is valid for.
	SetTags(mt names.ModelTag, ct names.ControllerTag)
	// GetUser returns the authenticated user.
	GetUser() names.UserTag
}

// WebsocketConnection represents the websocket connection interface used by the proxy.
type WebsocketConnection interface {
	ReadJSON(v any) error
	WriteJSON(v any) error
	Close() error
}

// WebsocketConnectionWithMetadata holds the websocket connection and metadata about the
// established connection.
type WebsocketConnectionWithMetadata struct {
	Conn           WebsocketConnection
	ControllerUUID string
	ModelName      string
	ModelUUID      string
	MigrationMode  dbmodel.MigrationMode
}

// LoginService represents the LoginService interface used by the proxy.
// Currently this is a duplicate of the [jujuapi.LoginService].
type LoginService interface {
	LoginDevice(ctx context.Context) (*oauth2.DeviceAuthResponse, error)
	GetDeviceSessionToken(ctx context.Context, deviceOAuthResponse *oauth2.DeviceAuthResponse) (string, error)
	LoginClientCredentials(ctx context.Context, clientID string, clientSecret string) (*openfga.User, error)
	LoginWithSessionToken(ctx context.Context, sessionToken string) (*openfga.User, error)
	LoginWithSessionCookie(ctx context.Context, identityID string) (*openfga.User, error)
}

// ProxyHelpers contains all the necessary helpers for proxying a Juju client
// connection to a model.
type ProxyHelpers struct {
	ConnClient              WebsocketConnection
	TokenGen                TokenGenerator
	ConnectController       func(context.Context) (WebsocketConnectionWithMetadata, error)
	AuditLog                func(*dbmodel.AuditLogEntry)
	LoginService            LoginService
	AuthenticatedIdentityID string
	RedirectInfo            RedirectInfoGetter
}

// ProxySockets will proxy requests from a client connection through to a controller
// tokenGen is used to authenticate the user and generate JWT token.
// connectController provides the function to return a connection to the desired controller endpoint.
func ProxySockets(ctx context.Context, helpers ProxyHelpers) error {
	session, err := newProxySession(helpers)
	if err != nil {
		return err
	}
	return session.run(ctx)
}

// writeLockConn provides a websocket connection that is safe for concurrent writes.
type writeLockConn struct {
	mu   sync.Mutex
	conn WebsocketConnection
}

// readJson allows for non-concurrent reads on the websocket.
func (c *writeLockConn) readJson(v any) error {
	return c.conn.ReadJSON(v)
}

// writeJson allows for concurrent writes on the websocket.
func (c *writeLockConn) writeJson(v any) error {
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
			if err := c.writeJson(errorMsg); err != nil {
				zapctx.Error(context.Background(), "failed to send error message in proxy", zap.Error(err))
			}

		}
		msg.Response = responseData
	}
	if err := c.writeJson(msg); err != nil {
		zapctx.Error(context.Background(), "failed to write message in proxy", zap.Error(err))
	}
}

type modelProxy struct {
	src                     *writeLockConn
	dst                     *writeLockConn
	inflight                *inflightTracker
	anonymousLogin          bool // anonymousLogin is true if the client is not authenticated.
	auditLog                func(*dbmodel.AuditLogEntry)
	tokenGen                TokenGenerator
	loginService            LoginService
	modelName               string
	modelUUID               string
	modelMigrationMode      dbmodel.MigrationMode
	conversationId          string
	authenticatedIdentityID string
	redirectInfo            RedirectInfoGetter

	deviceOAuthResponse *oauth2.DeviceAuthResponse
}

func (p *modelProxy) sendError(ctx context.Context, socket *writeLockConn, req *message, err error) {
	if req == nil {
		// If there was no message to error on, just return.
		return
	}
	if errors.ErrorCode(err) == errors.CodeUnauthorized {
		logger.LogUnauthorizedAccess(
			ctx,
			p.tokenGen.GetUser().String(),
			fmt.Sprintf("unauthorized access in model proxy for model %s", p.modelUUID),
		)
	}
	msg := createErrResponse(err, req)
	if msg != nil {
		if err := socket.writeJson(msg); err != nil {
			zapctx.Error(context.Background(), "failed to create err response message", zap.Error(err))
		}
	}
	// An error message is a response back to the client.
	servermon.JujuCallErrorCount.WithLabelValues(req.Type, req.Request, p.inflight.controllerID())
	if err := p.auditLogMessage(msg, true); err != nil {
		zapctx.Error(context.Background(), "failed to audit log message", zap.Error(err))
	}
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
		var allErrors jujuparams.ErrorResults
		if msg.Response != nil {
			err := json.Unmarshal(msg.Response, &allErrors)
			if err != nil {
				return fmt.Errorf("failed to unmarshal message response: %w", err)
			}
		}
		singleError := jujuparams.ErrorResult{Error: &jujuparams.Error{Message: msg.Error, Code: msg.ErrorCode, Info: msg.ErrorInfo}}
		allErrors.Results = append(allErrors.Results, singleError)
		jsonErr, err := json.Marshal(allErrors)
		if err != nil {
			return fmt.Errorf("failed to marshal all errors: %w", err)
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

func unexpectedReadError(err error) bool {
	if websocket.IsUnexpectedCloseError(err,
		websocket.CloseNormalClosure,
		websocket.CloseNoStatusReceived,
		websocket.CloseAbnormalClosure) {
		return true
	}
	_, unmarshalError := err.(*json.InvalidUnmarshalError)
	return unmarshalError
}

// clientProxy proxies messages from client->controller.
type clientProxy struct {
	modelProxy
	wg                   sync.WaitGroup
	errChan              chan error
	createControllerConn func(context.Context) (WebsocketConnectionWithMetadata, error)
	connectController    sync.Once
}

// start begins the client->controller proxier.
func (p *clientProxy) start(ctx context.Context) error {
	loginDispatcher := newAdminLoginDispatcher(p)
	defer func() {
		if p.dst != nil {
			p.dst.conn.Close()
		}
	}()
	for {
		msg := new(message)
		if err := p.src.readJson(&msg); err != nil {
			if unexpectedReadError(err) {
				zapctx.Error(ctx, "unexpected client read error", zap.Error(err))
				return err
			}
			return nil
		}
		// TODO: In some scenarios we don't need to ever dial the controller.
		// For example, if the client is only sending requests that are handled by JIMM
		// itself, like login or scenarios where JIMM returns a redirect.
		// But we currently need `makeControllerConnection` to set the model UUID and name
		// so that we can generate JWTs when messages DO need to be forwarded.
		// Refactor this to be avoid dialling the controller if we don't need to.
		err := p.makeControllerConnection(ctx)
		if err != nil {
			zapctx.Error(ctx, "error connecting to controller", zap.Error(err))
			p.sendError(ctx, p.src, msg, err)
			return fmt.Errorf("failed to connect to controller: %w", err)
		}
		if err := p.auditLogMessage(msg, false); err != nil {
			zapctx.Error(ctx, "failed to audit log message", zap.Error(err))
		}
		// All requests should be proxied as transparently as possible through to the controller
		// except for auth related requests like Login because JIMM is auth gateway.
		if msg.Type == "Admin" {
			toClient, toController, err := loginDispatcher.handle(ctx, msg)
			if err != nil {
				p.sendError(ctx, p.src, msg, err)
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
				p.inflight.rememberLogin(toController)
			}
		}
		p.inflight.track(msg)
		if err := p.dst.writeJson(msg); err != nil {
			zapctx.Error(ctx, "clientProxy error writing to dst", zap.Error(err))
			p.sendError(ctx, p.src, msg, err)
			p.inflight.finish(msg.RequestID)
			continue
		}
	}
}

// makeControllerConnection dials a controller and starts a go routine for
// proxying requests from the controller to the client.
func (p *clientProxy) makeControllerConnection(ctx context.Context) error {
	var createConnErr error
	// Create the controller connection once.
	p.connectController.Do(func() {
		createConnErr = p.attachController(ctx)
	})
	return createConnErr
}

func (p *clientProxy) attachController(ctx context.Context) error {
	connWithMetadata, err := p.createControllerConn(ctx)
	if err != nil {
		return err
	}

	p.inflight.setControllerUUID(connWithMetadata.ControllerUUID)
	p.modelName = connWithMetadata.ModelName
	p.modelUUID = connWithMetadata.ModelUUID
	p.modelMigrationMode = connWithMetadata.MigrationMode
	p.dst = &writeLockConn{conn: connWithMetadata.Conn}
	p.startControllerProxy(ctx)
	return nil
}

func (p *clientProxy) startControllerProxy(ctx context.Context) {
	controllerToClient := controllerProxy{
		modelProxy: modelProxy{
			src:                p.dst,
			dst:                p.src,
			inflight:           p.inflight,
			auditLog:           p.auditLog,
			tokenGen:           p.tokenGen,
			modelName:          p.modelName,
			conversationId:     p.conversationId,
			modelMigrationMode: p.modelMigrationMode,
		},
	}
	p.wg.Go(func() {
		p.errChan <- controllerToClient.start(ctx)
	})
}

// controllerProxy proxies messages from controller->client with the caveat that
// it will retry client->controller messages that require further permissions.
type controllerProxy struct {
	modelProxy
}

// start implements the controller->client proxier.
func (p *controllerProxy) start(ctx context.Context) error {
	responseHandler := newControllerResponseHandler(p)
	for {
		msg := new(message)
		if err := p.src.readJson(msg); err != nil {
			if unexpectedReadError(err) {
				zapctx.Error(ctx, "unexpected controller read error", zap.Error(err))
				return err
			}
			return nil
		}

		if err := responseHandler.handle(ctx, msg); err != nil {
			return err
		}
	}
}

func createErrResponse(err error, req *message) *message {
	errMsg := new(message)
	errMsg.RequestID = req.RequestID
	errMsg.ErrorInfo = errors.ErrorInfo(err)
	errMsg.Error = err.Error()
	errMsg.ErrorCode = string(errors.ErrorCode(err))
	return errMsg
}
