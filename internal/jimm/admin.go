// Copyright 2024 Canonical Ltd.

package jimm

import (
	"context"

	"golang.org/x/oauth2"

	"github.com/canonical/jimm/internal/errors"
	"github.com/canonical/jimm/internal/jimm/credentials"
)

// LoginDevice starts the device login flow.
func LoginDevice(ctx context.Context, authenticator OAuthAuthenticator) (*oauth2.DeviceAuthResponse, error) {
	const op = errors.Op("jujuapi.LoginDevice")

	deviceResponse, err := authenticator.Device(ctx)
	if err != nil {
		return nil, errors.E(op, err)
	}

	return deviceResponse, nil
}

func GetDeviceSessionToken(ctx context.Context, authenticator OAuthAuthenticator, credentialStore credentials.CredentialStore, deviceOAuthResponse *oauth2.DeviceAuthResponse) (string, error) {
	const op = errors.Op("jujuapi.GetDeviceSessionToken")

	if authenticator == nil {
		return "", errors.E("nil authenticator")
	}

	if credentialStore == nil {
		return "", errors.E("nil credential store")
	}

	token, err := authenticator.DeviceAccessToken(ctx, deviceOAuthResponse)
	if err != nil {
		return "", errors.E(op, err)
	}

	idToken, err := authenticator.ExtractAndVerifyIDToken(ctx, token)
	if err != nil {
		return "", errors.E(op, err)
	}

	email, err := authenticator.Email(idToken)
	if err != nil {
		return "", errors.E(op, err)
	}

	if err := authenticator.UpdateIdentity(ctx, email, token); err != nil {
		return "", errors.E(op, err)
	}

	encToken, err := authenticator.MintSessionToken(email)
	if err != nil {
		return "", errors.E(op, err)
	}

	return string(encToken), nil
}
