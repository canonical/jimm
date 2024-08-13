package mocks

import (
	"context"

	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/openfga"
	"golang.org/x/oauth2"
)

type LoginService struct {
	LoginDevice_            func(ctx context.Context) (*oauth2.DeviceAuthResponse, error)
	GetDeviceSessionToken_  func(ctx context.Context, deviceOAuthResponse *oauth2.DeviceAuthResponse) (string, error)
	LoginClientCredentials_ func(ctx context.Context, clientID string, clientSecret string) (*openfga.User, error)
	LoginWithSessionToken_  func(ctx context.Context, sessionToken string) (*openfga.User, error)
	LoginWithSessionCookie_ func(ctx context.Context, identityID string) (*openfga.User, error)
}

func (j *LoginService) LoginDevice(ctx context.Context) (*oauth2.DeviceAuthResponse, error) {
	if j.LoginDevice_ == nil {
		return nil, errors.E(errors.CodeNotImplemented)
	}
	return j.LoginDevice_(ctx)
}

func (j *LoginService) GetDeviceSessionToken(ctx context.Context, deviceOAuthResponse *oauth2.DeviceAuthResponse) (string, error) {
	if j.GetDeviceSessionToken_ == nil {
		return "", errors.E(errors.CodeNotImplemented)
	}
	return j.GetDeviceSessionToken_(ctx, deviceOAuthResponse)
}

func (j *LoginService) LoginClientCredentials(ctx context.Context, clientID string, clientSecret string) (*openfga.User, error) {
	if j.LoginClientCredentials_ == nil {
		return nil, errors.E(errors.CodeNotImplemented)
	}
	return j.LoginClientCredentials_(ctx, clientID, clientSecret)
}

func (j *LoginService) LoginWithSessionToken(ctx context.Context, sessionToken string) (*openfga.User, error) {
	if j.LoginWithSessionToken_ == nil {
		return nil, errors.E(errors.CodeNotImplemented)
	}
	return j.LoginWithSessionToken_(ctx, sessionToken)
}

func (j *LoginService) LoginWithSessionCookie(ctx context.Context, identityID string) (*openfga.User, error) {
	if j.LoginWithSessionCookie_ == nil {
		return nil, errors.E(errors.CodeNotImplemented)
	}
	return j.LoginWithSessionCookie_(ctx, identityID)
}
