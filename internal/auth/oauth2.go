// Copyright 2024 canonical.

package auth

import (
	"context"
	"net/mail"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/juju/zaputil/zapctx"
	"github.com/lestrrat-go/jwx/v2/jwa"
	"github.com/lestrrat-go/jwx/v2/jwt"
	"go.uber.org/zap"
	"golang.org/x/oauth2"

	"github.com/canonical/jimm/internal/errors"
)

// AuthenticationService handles authentication within JIMM.
type AuthenticationService struct {
	deviceConfig oauth2.Config
	// provider holds a OIDC provider wrapper for the OAuth2.0 /x/oauth package,
	// enabling UserInfo calls, wellknown retrieval and jwks verification.
	provider *oidc.Provider
	// sessionTokenExpiry holds the expiry time for JIMM minted access tokens (JWTs).
	sessionTokenExpiry time.Duration
}

// AuthenticationServiceParams holds the parameters to initialise
// an Authentication Service.
type AuthenticationServiceParams struct {
	// IssuerURL is the URL of the OAuth2.0 server.
	// I.e., http://localhost:8082/realms/jimm in the case of keycloak.
	IssuerURL string
	// DeviceClientID holds the OAuth2.0 client id registered and configured
	// to handle device OAuth2.0 flows. The client is NOT expected to be confidential
	// and as such does not need a client secret (given it is configured correctly).
	DeviceClientID string
	// DeviceScopes holds the scopes that you wish to retrieve.
	DeviceScopes []string
	// SessionTokenExpiry holds the expiry time of minted JIMM access tokens (JWTs).
	SessionTokenExpiry time.Duration
}

// NewAuthenticationService returns a new authentication service for handling
// authentication within JIMM.
func NewAuthenticationService(ctx context.Context, params AuthenticationServiceParams) (*AuthenticationService, error) {
	const op = errors.Op("auth.NewAuthenticationService")

	provider, err := oidc.NewProvider(ctx, params.IssuerURL)
	if err != nil {
		zapctx.Error(ctx, "failed to create oidc provider", zap.Error(err))
		return nil, errors.E(op, errors.CodeServerConfiguration, err, "failed to create oidc provider")
	}

	return &AuthenticationService{
		provider: provider,
		deviceConfig: oauth2.Config{
			ClientID: params.DeviceClientID,
			Endpoint: provider.Endpoint(),
			Scopes:   params.DeviceScopes,
		},
		sessionTokenExpiry: params.SessionTokenExpiry,
	}, nil
}

// Device initiates a device flow login and is step ONE of TWO.
//
// This is done via retrieving a:
// - Device code
// - User code
// - VerificationURI
// - Interval
// - Expiry
// From the device /auth endpoint.
//
// The verification uri and user code is sent to the user, as they must enter the code
// into the uri.
//
// The interval, expiry and device code and used to poll the token endpoint for completion.
func (as *AuthenticationService) Device(ctx context.Context) (*oauth2.DeviceAuthResponse, error) {
	const op = errors.Op("auth.AuthenticationService.Device")

	resp, err := as.deviceConfig.DeviceAuth(ctx)
	if err != nil {
		zapctx.Error(ctx, "device auth call failed", zap.Error(err))
		return nil, errors.E(op, err, "device auth call failed")
	}

	return resp, nil
}

// DeviceAccessToken continues and collect an access token during the device login flow
// and is step TWO.
//
// See Device(...) godoc for more info pertaining to the flow.
func (as *AuthenticationService) DeviceAccessToken(ctx context.Context, res *oauth2.DeviceAuthResponse) (*oauth2.Token, error) {
	const op = errors.Op("auth.AuthenticationService.DeviceAccessToken")

	t, err := as.deviceConfig.DeviceAccessToken(ctx, res)
	if err != nil {
		return nil, errors.E(op, err, "device access token call failed")
	}

	return t, nil
}

// ExtractAndVerifyIDToken extracts the id token from the extras claims of an oauth2 token
// and performs signature verification of the token.
func (as *AuthenticationService) ExtractAndVerifyIDToken(ctx context.Context, oauth2Token *oauth2.Token) (*oidc.IDToken, error) {
	const op = errors.Op("auth.AuthenticationService.ExtractAndVerifyIDToken")

	// Extract the ID Token from oauth2 token.
	rawIDToken, ok := oauth2Token.Extra("id_token").(string)
	if !ok {
		return nil, errors.E(op, "failed to extract id token")
	}

	verifier := as.provider.Verifier(&oidc.Config{
		ClientID: as.deviceConfig.ClientID,
	})

	token, err := verifier.Verify(ctx, rawIDToken)
	if err != nil {
		return nil, errors.E(op, err, "failed to verify id token")
	}

	return token, nil
}

// Email retrieves the users email from an id token via the email claim
func (as *AuthenticationService) Email(idToken *oidc.IDToken) (string, error) {
	const op = errors.Op("auth.AuthenticationService.Email")

	var claims struct {
		Email         string `json:"email"`
		EmailVerified bool   `json:"email_verified"` // TODO(ale8k): Add verification logic
	}
	if idToken == nil {
		return "", errors.E(op, "id token is nil")
	}

	if err := idToken.Claims(&claims); err != nil {
		return "", errors.E(op, err, "failed to extract claims")
	}

	return claims.Email, nil
}

// MintSessionToken mints a session token to be used when logging into JIMM
// via an access token. The token only contains the user's email for authentication.
func (as *AuthenticationService) MintSessionToken(email string, secretKey string) ([]byte, error) {
	const op = errors.Op("auth.AuthenticationService.MintAccessToken")

	token, err := jwt.NewBuilder().
		Subject(email).
		Expiration(time.Now().Add(as.sessionTokenExpiry)).
		Build()
	if err != nil {
		return nil, errors.E(op, err, "failed to build access token")
	}

	freshToken, err := jwt.Sign(token, jwt.WithKey(jwa.HS256, []byte(secretKey)))
	if err != nil {
		return nil, errors.E(op, err, "failed to sign access token")
	}
	return freshToken, nil
}

// VerifyAccessToken symmetrically verifies the validty of the signature on the
// access token JWT, returning the parsed token.
//
// The subject of the token contains the user's email and can be used
// for user object creation.
func (as *AuthenticationService) VerifyAccessToken(token []byte, secretKey string) (jwt.Token, error) {
	const op = errors.Op("auth.AuthenticationService.VerifyAccessToken")

	parsedToken, err := jwt.Parse(token, jwt.WithKey(jwa.HS256, []byte(secretKey)))
	if err != nil {
		return nil, errors.E(op, err)
	}

	if _, err = mail.ParseAddress(parsedToken.Subject()); err != nil {
		return nil, errors.E(op, "failed to parse email")
	}

	return parsedToken, nil
}
