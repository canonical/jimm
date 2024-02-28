// Copyright 2024 canonical.

package auth

import (
	"context"
	"encoding/base64"
	stderrors "errors"
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
	oauthConfig oauth2.Config
	// provider holds a OIDC provider wrapper for the OAuth2.0 /x/oauth package,
	// enabling UserInfo calls, wellknown retrieval and jwks verification.
	provider *oidc.Provider
	// sessionTokenExpiry holds the expiry time for JIMM minted session tokens (JWTs).
	sessionTokenExpiry time.Duration
}

// AuthenticationServiceParams holds the parameters to initialise
// an Authentication Service.
type AuthenticationServiceParams struct {
	// IssuerURL is the URL of the OAuth2.0 server.
	// I.e., http://localhost:8082/realms/jimm in the case of keycloak.
	IssuerURL string
	// ClientID holds the OAuth2.0 client id. The client IS expected to be confidential.
	ClientID string
	// ClientSecret holds the OAuth2.0 "client-secret" to authenticate when performing
	// /auth and /token requests.
	ClientSecret string
	// Scopes holds the scopes that you wish to retrieve.
	Scopes []string
	// SessionTokenExpiry holds the expiry time of minted JIMM session tokens (JWTs).
	SessionTokenExpiry time.Duration
	// RedirectURL is the URL for handling the exchange of authorisation
	// codes into access tokens (and id tokens), for JIMM, this is expected
	// to be the servers own callback endpoint registered under /auth/callback.
	RedirectURL string
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
		oauthConfig: oauth2.Config{
			ClientID:     params.ClientID,
			ClientSecret: params.ClientSecret,
			Endpoint:     provider.Endpoint(),
			Scopes:       params.Scopes,
			RedirectURL:  params.RedirectURL,
		},
		sessionTokenExpiry: params.SessionTokenExpiry,
	}, nil
}

// AuthCodeURL returns a URL that will be used to redirect a browser to the identity provider.
func (as *AuthenticationService) AuthCodeURL() string {
	// As we're not the browser creating the auth code url and then communicating back
	// to the server, it is OK not to set a state as there's no communication
	// between say many "tabs" and a JIMM deployment, but rather
	// just JIMM creating the auth code URL itself, and then handling the exchanging
	// itself. Of course, middleman attacks between the IdP and JIMM are possible,
	// but we'd have much larger problems than an auth code interception at that
	// point. As such, we're opting out of using auth code URL state.
	return as.oauthConfig.AuthCodeURL("")
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

	resp, err := as.oauthConfig.DeviceAuth(
		ctx,
		oauth2.SetAuthURLParam("client_secret", as.oauthConfig.ClientSecret),
	)
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

	t, err := as.oauthConfig.DeviceAccessToken(
		ctx,
		res,
		oauth2.SetAuthURLParam("client_secret", as.oauthConfig.ClientSecret),
	)
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
		ClientID: as.oauthConfig.ClientID,
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
func (as *AuthenticationService) MintSessionToken(email string, secretKey string) (string, error) {
	const op = errors.Op("auth.AuthenticationService.MintAccessToken")

	token, err := jwt.NewBuilder().
		Subject(email).
		Expiration(time.Now().Add(as.sessionTokenExpiry)).
		Build()
	if err != nil {
		return "", errors.E(op, err, "failed to build access token")
	}

	freshToken, err := jwt.Sign(token, jwt.WithKey(jwa.HS256, []byte(secretKey)))
	if err != nil {
		return "", errors.E(op, err, "failed to sign access token")
	}

	return base64.StdEncoding.EncodeToString(freshToken), nil
}

// VerifySessionToken calls the exported VerifySessionToken function.
func (as *AuthenticationService) VerifySessionToken(token string, secretKey string) (jwt.Token, error) {
	return VerifySessionToken(token, secretKey)
}

// VerifySessionToken symmetrically verifies the validty of the signature on the
// access token JWT, returning the parsed token.
//
// The subject of the token contains the user's email and can be used
// for user object creation
//
// This method is exported for use by the mock authenticator.
func VerifySessionToken(token string, secretKey string) (jwt.Token, error) {
	const op = errors.Op("auth.AuthenticationService.VerifySessionToken")

	if len(token) == 0 {
		return nil, errors.E(op, "authentication failed, no token presented")
	}

	decodedToken, err := base64.StdEncoding.DecodeString(token)
	if err != nil {
		return nil, errors.E(op, "authentication failed, failed to decode token")
	}

	parsedToken, err := jwt.Parse(decodedToken, jwt.WithKey(jwa.HS256, []byte(secretKey)))
	if err != nil {
		if stderrors.Is(err, jwt.ErrTokenExpired()) {
			return nil, errors.E(op, "JIMM session token expired")
		}
		return nil, errors.E(op, err)
	}

	if _, err = mail.ParseAddress(parsedToken.Subject()); err != nil {
		return nil, errors.E(op, "failed to parse email")
	}

	return parsedToken, nil
}
