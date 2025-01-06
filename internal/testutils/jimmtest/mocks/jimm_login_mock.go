// Copyright 2025 Canonical.
package mocks

import (
	"context"
	"net/http"

	"golang.org/x/oauth2"

	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/openfga"
)

type LoginManager struct {
	AuthenticateBrowserSession_ func(ctx context.Context, w http.ResponseWriter, req *http.Request) (context.Context, error)
	LoginDevice_                func(ctx context.Context) (*oauth2.DeviceAuthResponse, error)
	GetDeviceSessionToken_      func(ctx context.Context, deviceOAuthResponse *oauth2.DeviceAuthResponse) (string, error)
	LoginClientCredentials_     func(ctx context.Context, clientID string, clientSecret string) (*openfga.User, error)
	LoginWithSessionToken_      func(ctx context.Context, sessionToken string) (*openfga.User, error)
	LoginWithSessionCookie_     func(ctx context.Context, identityID string) (*openfga.User, error)
	UserLogin_                  func(ctx context.Context, identityName string) (*openfga.User, error)
}

func (j *LoginManager) AuthenticateBrowserSession(ctx context.Context, w http.ResponseWriter, req *http.Request) (context.Context, error) {
	if j.AuthenticateBrowserSession_ == nil {
		return nil, errors.E(errors.CodeNotImplemented)
	}
	return j.AuthenticateBrowserSession_(ctx, w, req)
}

func (j *LoginManager) LoginDevice(ctx context.Context) (*oauth2.DeviceAuthResponse, error) {
	if j.LoginDevice_ == nil {
		return nil, errors.E(errors.CodeNotImplemented)
	}
	return j.LoginDevice_(ctx)
}

func (j *LoginManager) GetDeviceSessionToken(ctx context.Context, deviceOAuthResponse *oauth2.DeviceAuthResponse) (string, error) {
	if j.GetDeviceSessionToken_ == nil {
		return "", errors.E(errors.CodeNotImplemented)
	}
	return j.GetDeviceSessionToken_(ctx, deviceOAuthResponse)
}

func (j *LoginManager) LoginClientCredentials(ctx context.Context, clientID string, clientSecret string) (*openfga.User, error) {
	if j.LoginClientCredentials_ == nil {
		return nil, errors.E(errors.CodeNotImplemented)
	}
	return j.LoginClientCredentials_(ctx, clientID, clientSecret)
}

func (j *LoginManager) LoginWithSessionToken(ctx context.Context, sessionToken string) (*openfga.User, error) {
	if j.LoginWithSessionToken_ == nil {
		return nil, errors.E(errors.CodeNotImplemented)
	}
	return j.LoginWithSessionToken_(ctx, sessionToken)
}

func (j *LoginManager) LoginWithSessionCookie(ctx context.Context, identityID string) (*openfga.User, error) {
	if j.LoginWithSessionCookie_ == nil {
		return nil, errors.E(errors.CodeNotImplemented)
	}
	return j.LoginWithSessionCookie_(ctx, identityID)
}

func (j *LoginManager) UserLogin(ctx context.Context, identityName string) (*openfga.User, error) {
	if j.UserLogin_ == nil {
		return nil, errors.E(errors.CodeNotImplemented)
	}
	return j.UserLogin_(ctx, identityName)
}
