// Copyright 2025 Canonical.

package rpcproxy

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"

	"github.com/juju/juju/api"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v5"
	"github.com/juju/zaputil/zapctx"
	"go.uber.org/zap"

	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/openfga"
	apiparams "github.com/canonical/jimm/v3/pkg/api/params"
)

// adminLoginDispatcher isolates Admin facade login handling from the proxy loop.
type adminLoginDispatcher struct {
	proxy *clientProxy
}

func newAdminLoginDispatcher(proxy *clientProxy) adminLoginDispatcher {
	return adminLoginDispatcher{proxy: proxy}
}

// handle processes the admin facade call and returns:
// a message to be returned to the source
// a message to be sent to the destination
// an error
func (h adminLoginDispatcher) handle(ctx context.Context, msg *message) (clientResponse *message, controllerMessage *message, err error) {
	errorFnc := func(err error) (*message, *message, error) {
		return nil, nil, err
	}
	controllerLoginMessageFnc := func(user *openfga.User) (*message, *message, error) {
		jwt, err := h.proxy.tokenGen.MakeLoginToken(ctx, user)
		if err != nil {
			return errorFnc(err)
		}
		data, err := json.Marshal(jujuparams.LoginRequest{
			AuthTag: names.NewUserTag(user.Name).String(),
			Token:   base64.StdEncoding.EncodeToString(jwt),
		})
		if err != nil {
			return errorFnc(err)
		}
		m := *msg
		m.Type = "Admin"
		m.Request = "Login"
		m.Version = 3
		m.Params = data
		return nil, &m, nil
	}

	switch msg.Request {
	case "LoginDevice":
		deviceResponse, err := h.proxy.loginService.LoginDevice(ctx)
		if err != nil {
			return errorFnc(err)
		}
		h.proxy.deviceOAuthResponse = deviceResponse

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
		sessionToken, err := h.proxy.loginService.GetDeviceSessionToken(ctx, h.proxy.deviceOAuthResponse)
		if err != nil {
			return errorFnc(err)
		}
		// #nosec G117 session token is sensitive but the data object is not logged.
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

		user, err := h.proxy.loginService.LoginWithSessionToken(ctx, request.SessionToken)
		if err != nil {
			return errorFnc(err)
		}

		return controllerLoginMessageFnc(user)
	case "LoginWithClientCredentials":
		var request apiparams.LoginWithClientCredentialsRequest
		err := json.Unmarshal(msg.Params, &request)
		if err != nil {
			return errorFnc(err)
		}
		user, err := h.proxy.loginService.LoginClientCredentials(ctx, request.ClientID, request.ClientSecret)
		if err != nil {
			return errorFnc(err)
		}

		return controllerLoginMessageFnc(user)
	case "LoginWithSessionCookie":
		user, err := h.proxy.loginService.LoginWithSessionCookie(ctx, h.proxy.authenticatedIdentityID)
		if err != nil {
			return errorFnc(err)
		}

		return controllerLoginMessageFnc(user)
	case "Login":
		controllerMessage, err := h.handleLegacyLogin(ctx, msg)
		return nil, controllerMessage, err
	default:
		return nil, nil, nil
	}
}

// handleLegacyLogin handles old style username+password/macaroon login
// requests and decides what to do based on the client's auth tag.
//
// If the request is an "anonymous login" request (i.e., the auth tag is
// api.AnonymousUsername), it sets the anonymousLogin flag to true and returns
// the message verbatim to the controller, allowing it to handle the login.
// This supports login from a Juju controller during model migration.
//
// If the auth tag is a non-user entity e.g. a machine/unit then we return
// a redirect to the backing Juju controller. This is also used for model migrations
// but specifically for directing agents to speak to the backing Juju controller.
//
// Legacy login requests from users (i.e., those with a user tag) are not supported
// in JIMM and will return an error.
func (h adminLoginDispatcher) handleLegacyLogin(ctx context.Context, msg *message) (*message, error) {
	var request jujuparams.LoginRequest
	err := json.Unmarshal(msg.Params, &request)
	if err != nil {
		return nil, err
	}
	tag, err := names.ParseTag(request.AuthTag)
	if err != nil {
		return nil, fmt.Errorf("invalid user tag: %v", err)
	}
	switch tag := tag.(type) {
	case names.UserTag:
		if tag.Id() == api.AnonymousUsername {
			h.proxy.anonymousLogin = true
			// return the client's login message verbatim to the controller.
			return msg, nil
		}
		return nil, errors.Codef(errors.CodeNotSupported, "JIMM does not support login from old clients")
	case names.ModelTag, names.MachineTag, names.UnitTag:
		zapctx.Debug(ctx, "Legacy login request from agent", zap.String("tag", tag.String()))

		redirectInfo, err := h.proxy.redirectInfo.GetRedirectInfo(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to get redirect info: %w", err)
		}

		// This is a legacy login request from an agent.
		// We return a redirect to the backing Juju controller.
		info := jujuparams.RedirectErrorInfo{
			Servers: redirectInfo.Addresses,
			CACert:  redirectInfo.CACert,
		}.AsMap()
		errRedirect := &errors.Error{
			Code:    errors.CodeRedirect,
			Message: "redirection to alternative server required",
			Info:    info,
		}

		zapctx.Debug(ctx, "Redirecting agent to controller", zap.Any("servers", redirectInfo.Addresses))
		return nil, errRedirect
	default:
		return nil, fmt.Errorf("unsupported login request for tag %s", tag)
	}
}
