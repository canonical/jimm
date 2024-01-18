// Copyright 2024 canonical.

package auth

import (
	"context"
	"errors"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

// AuthenticationService handles authentication within JIMM.
type AuthenticationService struct {
	deviceConfig oauth2.Config
	// provider holds a OIDC provider wrapper for the OAuth2.0 /x/oauth package,
	// enabling UserInfo calls, wellknown retrieval and jwks verification.
	provider *oidc.Provider
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
}

// NewAuthenticationService returns a new authentication service for handling
// authentication within JIMM.
func NewAuthenticationService(ctx context.Context, params AuthenticationServiceParams) (*AuthenticationService, error) {
	provider, err := oidc.NewProvider(ctx, params.IssuerURL)
	if err != nil {
		return nil, err
	}

	return &AuthenticationService{
		provider: provider,
		deviceConfig: oauth2.Config{
			ClientID: params.DeviceClientID,
			Endpoint: provider.Endpoint(),
			Scopes:   params.DeviceScopes,
		},
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
	return as.deviceConfig.DeviceAuth(ctx)
}

// DeviceAccessToken continues and collect an access token during the device login flow
// and is step TWO.
//
// See Device(...) godoc for more info pertaining to the fko.
func (as *AuthenticationService) DeviceAccessToken(ctx context.Context, res *oauth2.DeviceAuthResponse) (*oauth2.Token, error) {
	return as.deviceConfig.DeviceAccessToken(ctx, res)
}

// ExtractAndVerifyIDToken extracts the id token from the extras claims of an oauth2 token
// and performs signature verification of the token.
func (as *AuthenticationService) ExtractAndVerifyIDToken(ctx context.Context, oauth2Token *oauth2.Token) (*oidc.IDToken, error) {
	// Extract the ID Token from oauth2 token.
	rawIDToken, ok := oauth2Token.Extra("id_token").(string)
	if !ok {
		return nil, errors.New("failed to extract id token")
	}

	verifier := as.provider.Verifier(&oidc.Config{
		ClientID: as.deviceConfig.ClientID,
	})

	token, err := verifier.Verify(ctx, rawIDToken)
	return token, err
}

// Email retrieves the users email from an id token via the email claim
func (as *AuthenticationService) Email(idToken *oidc.IDToken) (string, error) {
	var claims struct {
		Email         string `json:"email"`
		EmailVerified bool   `json:"email_verified"` // TODO(ale8k): Add verification logic
	}
	if idToken == nil {
		return "", errors.New("id token is nil")
	}
	err := idToken.Claims(&claims)
	return claims.Email, err

}
