// Copyright 2024 Canonical.

package jimm

import (
	"context"
	"net/http"

	"golang.org/x/oauth2"

	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/openfga"
	"github.com/canonical/jimm/v3/pkg/names"
)

// LoginDevice starts the device login flow.
func (j *JIMM) LoginDevice(ctx context.Context) (*oauth2.DeviceAuthResponse, error) {
	const op = errors.Op("jimm.LoginDevice")
	resp, err := j.OAuthAuthenticator.Device(ctx)
	if err != nil {
		return nil, errors.E(op, err)
	}
	return resp, nil
}

// AuthenticateBrowserSession authenticates a browser login.
func (j *JIMM) AuthenticateBrowserSession(ctx context.Context, w http.ResponseWriter, r *http.Request) (context.Context, error) {
	return j.OAuthAuthenticator.AuthenticateBrowserSession(ctx, w, r)
}

// GetDeviceSessionToken polls an OIDC server while a user logs in and returns a session token scoped to the user's identity.
func (j *JIMM) GetDeviceSessionToken(ctx context.Context, deviceOAuthResponse *oauth2.DeviceAuthResponse) (string, error) {
	const op = errors.Op("jimm.GetDeviceSessionToken")

	token, err := j.OAuthAuthenticator.DeviceAccessToken(ctx, deviceOAuthResponse)
	if err != nil {
		return "", errors.E(op, err)
	}

	idToken, err := j.OAuthAuthenticator.ExtractAndVerifyIDToken(ctx, token)
	if err != nil {
		return "", errors.E(op, err)
	}

	email, err := j.OAuthAuthenticator.Email(idToken)
	if err != nil {
		return "", errors.E(op, err)
	}

	if err := j.OAuthAuthenticator.UpdateIdentity(ctx, email, token); err != nil {
		return "", errors.E(op, err)
	}

	encToken, err := j.OAuthAuthenticator.MintSessionToken(email)
	if err != nil {
		return "", errors.E(op, err)
	}

	return string(encToken), nil
}

// LoginClientCredentials verifies a user's client ID and secret before the user is logged in.
func (j *JIMM) LoginClientCredentials(ctx context.Context, clientID string, clientSecret string) (*openfga.User, error) {
	const op = errors.Op("jimm.LoginClientCredentials")
	// We expect the client to send the service account ID "as-is" and because we know that this is a clientCredentials login,
	// we can append the @serviceaccount domain to the clientID (if not already present).
	clientIdWithDomain, err := names.EnsureValidServiceAccountId(clientID)
	if err != nil {
		return nil, errors.E(op, err)
	}

	err = j.OAuthAuthenticator.VerifyClientCredentials(ctx, clientID, clientSecret)
	if err != nil {
		return nil, errors.E(op, err)
	}

	return j.UserLogin(ctx, clientIdWithDomain)
}

// LoginWithSessionToken verifies a user's session token before the user is logged in.
func (j *JIMM) LoginWithSessionToken(ctx context.Context, sessionToken string) (*openfga.User, error) {
	const op = errors.Op("jimm.LoginWithSessionToken")
	jwtToken, err := j.OAuthAuthenticator.VerifySessionToken(sessionToken)
	if err != nil {
		return nil, errors.E(op, err)
	}

	email := jwtToken.Subject()
	return j.UserLogin(ctx, email)
}

// LoginWithSessionCookie uses the identity ID expected to have come from a session cookie, to log the user in.
//
// The work to parse and store the user's identity from the session cookie takes place in internal/jimmhttp/websocket.go
// [WSHandler.ServerHTTP] during the upgrade from an HTTP connection to a websocket. The user's identity is stored
// and passed to this function with the assumption that the cookie contained a valid session. This function is far from
// the session cookie logic due to the separation between the HTTP layer and Juju's RPC mechanism.
func (j *JIMM) LoginWithSessionCookie(ctx context.Context, identityID string) (*openfga.User, error) {
	const op = errors.Op("jimm.LoginWithSessionCookie")
	if identityID == "" {
		return nil, errors.E(op, "missing cookie identity")
	}
	return j.UserLogin(ctx, identityID)
}
