// Copyright 2025 Canonical.

package rpcproxy

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"

	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/zaputil/zapctx"
	"go.uber.org/zap"

	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
)

// controllerResponseHandler keeps controller->client response handling in one place
// so the proxy loop stays focused on transport orchestration.
type controllerResponseHandler struct {
	proxy *controllerProxy
}

func newControllerResponseHandler(proxy *controllerProxy) controllerResponseHandler {
	return controllerResponseHandler{proxy: proxy}
}

func (h controllerResponseHandler) handle(ctx context.Context, msg *message) error {
	if !h.processControllerErrors(ctx, msg) {
		return nil
	}

	if err := modifyControllerResponse(msg); err != nil {
		zapctx.Error(ctx, "Failed to modify message", zap.Error(err))
		h.handleError(ctx, msg, err)
		return fmt.Errorf("error modifying controller response: %w", err)
	}
	h.proxy.msgs.removeMessage(msg.RequestID)
	if err := h.proxy.auditLogMessage(msg, true); err != nil {
		zapctx.Error(context.Background(), "failed to audit log message", zap.Error(err))
	}
	if err := h.proxy.dst.writeJson(msg); err != nil {
		zapctx.Error(ctx, "controllerProxy error writing to dst", zap.Error(err))
		return fmt.Errorf("error writing message to client: %w", err)
	}
	return nil
}

// processControllerErrors checks for errors in the message from the controller
// and decides how to handle them. It returns true if the message should be
// returned to the client, false if it should not.
func (h controllerResponseHandler) processControllerErrors(ctx context.Context, msg *message) bool {
	// Check if the model is migrating. When it has completed its migration, we will receive
	// an unauthorized error from the controller and we want to mask that error and inform
	// clients to try again as JIMM will eventually update the model's controller.
	// See internal/jimm/juju/model_poller.go.
	modelMigrating := h.proxy.modelMigrationMode == dbmodel.MigrationModeMigrateInternal || h.proxy.modelMigrationMode == dbmodel.MigrationModeExporting
	if modelMigrating && msg.ErrorCode == string(errors.CodeUnauthorized) {
		msg.ErrorCode = string(errors.CodeModelMigrating)
		msg.Error = "model is finishing migration, please retry later"
		return true
	}

	// Next we check for permission required errors where the Juju controller is informing us
	// that the user needs more permissions to perform the requested operation.
	// If so, we attempt to redo the login with the required permissions (if the user has them)
	// and then resend the original login request.
	//
	// It's ideally unlikely we'll hit this code path often because JIMM sets a broad scope
	// of permissions on the initial login request.
	permissionsRequired, err := checkPermissionsRequired(ctx, msg)
	if err != nil {
		zapctx.Error(ctx, "failed to determine if more permissions required", zap.Error(err))
		h.handleError(ctx, msg, err)
		return false
	}
	if permissionsRequired != nil {
		zapctx.Error(ctx, "Access Required error")
		if err := h.redoLogin(ctx, permissionsRequired); err != nil {
			zapctx.Error(ctx, "Failed to redo login", zap.Error(err))
			h.handleError(ctx, msg, err)
			return false
		}
		// Write back to the controller.
		msg := h.proxy.msgs.getMessage(msg.RequestID)
		if msg != nil {
			if err := h.proxy.src.writeJson(msg); err != nil {
				zapctx.Error(context.Background(), "failed to write back to controller", zap.Error(err))
			}
		}
		return false
	}
	return true
}

func (h controllerResponseHandler) handleError(ctx context.Context, msg *message, err error) {
	h.proxy.sendError(ctx, h.proxy.dst, msg, err)
	h.proxy.msgs.removeMessage(msg.RequestID)
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

	var er jujuparams.ErrorResults
	err := json.Unmarshal(msg.Response, &er)
	if err != nil {
		zapctx.Error(ctx, "failed to read response error", zap.Error(err))
		return permissionMap, nil
	}

	// Check for errors that may be a result of a bulk request.
	for _, e := range er.Results {
		if e.Error != nil && e.Error.Code == accessRequiredErrorCode {
			zapctx.Debug(ctx, "received error", zap.Any("error", e.Error))
			for k, v := range e.Error.Info {
				accessLevel, ok := v.(string)
				if !ok {
					return nil, errors.New("unknown permission level")
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

// redoLogin sends a new login request to the controller after checking for
// the provided permissions. This is sometimes necessary if Juju requires
// extra permission checks for an operation. If the client performed anonymous
// login, an error is always returned since we cannot authorize an anonymous user.
func (h controllerResponseHandler) redoLogin(ctx context.Context, permissions map[string]any) error {
	if h.proxy.anonymousLogin {
		return errors.Codef(errors.CodeUnauthorized, "Anonymous login does not support re-authentication")
	}
	loginMsg := h.proxy.msgs.getLoginMessage()
	if loginMsg == nil {
		return errors.Codef(errors.CodeUnauthorized, "Haven't received login yet")
	}
	err := addJWT(ctx, loginMsg, permissions, h.proxy.tokenGen)
	if err != nil {
		return err
	}
	zapctx.Info(ctx, "Performing new login", zap.Any("message", loginMsg))
	if err := h.proxy.src.writeJson(loginMsg); err != nil {
		return err
	}
	return nil
}

// addJWT adds a JWT token to the the provided message.
func addJWT(ctx context.Context, msg *message, permissions map[string]any, tokenGen TokenGenerator) error {
	// First we unmarshal the existing LoginRequest.
	if msg == nil {
		return errors.New("nil messsage")
	}
	var lr jujuparams.LoginRequest
	if err := json.Unmarshal(msg.Params, &lr); err != nil {
		return err
	}

	jwt, err := tokenGen.MakeToken(ctx, permissions)
	if err != nil {
		return err
	}

	jwtString := base64.StdEncoding.EncodeToString(jwt)
	// Add the JWT as base64 encoded string.
	lr.Token = jwtString
	// Marshal it again to JSON.
	data, err := json.Marshal(lr)
	if err != nil {
		return err
	}
	// And add it to the message.
	msg.Params = data
	return nil
}

func modifyControllerResponse(msg *message) error {
	var response map[string]any
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
