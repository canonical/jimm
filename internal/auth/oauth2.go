// Copyright 2024 canonical.

// Package auth provides means to authenticate users into JIMM.
//
// The methods of authentication are:
// - OAuth2.0 (Device flow)
// - OAuth2.0 (Browser flow)
// - JWTs (For CLI based sessions)
package auth

import (
	"context"
	"encoding/base64"
	stderrors "errors"
	"fmt"
	"net/http"
	"net/mail"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/gorilla/sessions"
	"github.com/juju/zaputil/zapctx"
	"github.com/lestrrat-go/jwx/v2/jwa"
	"github.com/lestrrat-go/jwx/v2/jwt"
	"go.uber.org/zap"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/clientcredentials"

	"github.com/canonical/jimm/internal/dbmodel"
	"github.com/canonical/jimm/internal/errors"
)

const (
	// SessionName is the name of the gorilla session and is used to retrieve
	// the session object from the database.
	SessionName = "jimm-browser-session"

	// SessionIdentityKey is the key for the identity value stored within the
	// session.
	SessionIdentityKey = "identity-id"
)

type sessionIdentityContextKey struct{}

func contextWithSessionIdentity(ctx context.Context, sessionIdentityId any) context.Context {
	return context.WithValue(ctx, sessionIdentityContextKey{}, sessionIdentityId)
}

// SessionIdentityFromContext returns the session identity key from the context.
func SessionIdentityFromContext(ctx context.Context) string {
	v := ctx.Value(sessionIdentityContextKey{})
	if v == nil {
		return ""
	}
	return v.(string)
}

// AuthenticationService handles authentication within JIMM.
type AuthenticationService struct {
	oauthConfig oauth2.Config
	// provider holds a OIDC provider wrapper for the OAuth2.0 /x/oauth package,
	// enabling UserInfo calls, wellknown retrieval and jwks verification.
	provider *oidc.Provider
	// sessionTokenExpiry holds the expiry time for JIMM minted session tokens (JWTs).
	sessionTokenExpiry time.Duration

	// sessionCookieMaxAge holds the max age for session cookies.
	sessionCookieMaxAge int

	db IdentityStore

	sessionStore sessions.Store
}

// Identity store holds the necessary methods to get and update an identity
// within JIMM's store.
type IdentityStore interface {
	GetIdentity(ctx context.Context, u *dbmodel.Identity) error
	UpdateIdentity(ctx context.Context, u *dbmodel.Identity) error
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
	// SessionCookieMaxAge holds the max age for session cookies.
	SessionCookieMaxAge int
	// RedirectURL is the URL for handling the exchange of authorisation
	// codes into access tokens (and id tokens), for JIMM, this is expected
	// to be the servers own callback endpoint registered under /auth/callback.
	RedirectURL string

	// Store holds the identity store used by the authentication service
	// to fetch and update identities. I.e., their access tokens, refresh tokens,
	// display name, etc.
	Store IdentityStore

	// SessionStore holds the store for creating, getting and saving gorrila sessions.
	SessionStore sessions.Store
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
		sessionTokenExpiry:  params.SessionTokenExpiry,
		db:                  params.Store,
		sessionStore:        params.SessionStore,
		sessionCookieMaxAge: params.SessionCookieMaxAge,
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

// Exchange exchanges an authorisation code for an access token.
//
// TODO(ale8k): How to test this? A callback has to be made and it needs to be valid,
// this may need some thought as to whether its actually worth testing or are we
// just testing the library. The handler test essentially covers this so perhaps
// its ok to leave it as is?
func (as *AuthenticationService) Exchange(ctx context.Context, code string) (*oauth2.Token, error) {
	const op = errors.Op("auth.AuthenticationService.Exchange")

	t, err := as.oauthConfig.Exchange(
		ctx,
		code,
		oauth2.SetAuthURLParam("client_secret", as.oauthConfig.ClientSecret),
	)
	if err != nil {
		return nil, errors.E(op, err, "authorisation code exchange failed")
	}

	return t, nil
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
		zapctx.Error(ctx, "failed to verify id token", zap.Error(err))
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

// UpdateIdentity updates the database with the display name and access token set for the user.
// And, if present, a refresh token.
func (as *AuthenticationService) UpdateIdentity(ctx context.Context, email string, token *oauth2.Token) error {
	const op = errors.Op("auth.UpdateIdentity")

	db := as.db
	u := &dbmodel.Identity{
		Name: email,
	}
	// TODO(babakks): If user does not exist, we will create one with an empty
	// display name (which we shouldn't). So it would be better to fetch
	// and then create. At the moment, GetUser is used for both create and fetch,
	// this should be changed and split apart so it is intentional what entities
	// we are creating or fetching.
	if err := db.GetIdentity(ctx, u); err != nil {
		return errors.E(op, err)
	}
	// Check if user has a display name, if not, set one
	if u.DisplayName == "" {
		splitEmail := strings.Split(email, "@")
		if len(splitEmail) > 0 {
			u.DisplayName = strings.Split(email, "@")[0]
		} else {
			return errors.E(op, "failed to split email")
		}
	}

	u.AccessToken = token.AccessToken
	u.RefreshToken = token.RefreshToken
	u.AccessTokenExpiry = token.Expiry
	u.AccessTokenType = token.TokenType
	if err := db.UpdateIdentity(ctx, u); err != nil {
		return errors.E(op, err)
	}

	return nil
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

// VerifyClientCredentials verifies the provided client ID and client secret.
func (as *AuthenticationService) VerifyClientCredentials(ctx context.Context, clientID string, clientSecret string) error {
	cfg := clientcredentials.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		TokenURL:     as.oauthConfig.Endpoint.TokenURL,
		Scopes:       as.oauthConfig.Scopes,
		AuthStyle:    oauth2.AuthStyle(as.oauthConfig.Endpoint.AuthStyle),
	}

	_, err := cfg.Token(ctx)
	if err != nil {
		zapctx.Error(ctx, "client credential verification failed", zap.Error(err))
		return errors.E(errors.CodeUnauthorized, "invalid client credentials")
	}
	return nil
}

// CreateBrowserSession creates a session and updates the cookie for a browser
// login callback.
func (as *AuthenticationService) CreateBrowserSession(
	ctx context.Context,
	w http.ResponseWriter,
	r *http.Request,
	secureCookies bool,
	email string,
) error {
	const op = errors.Op("auth.AuthenticationService.CreateBrowserSession")

	session, err := as.sessionStore.Get(r, SessionName)
	if err != nil {
		return errors.E(op, err)
	}

	session.IsNew = true                            // Sets cookie to a fresh new cookie
	session.Options.MaxAge = as.sessionCookieMaxAge // Expiry in seconds
	session.Options.Secure = secureCookies          // Ensures only sent with HTTPS
	session.Options.HttpOnly = false                // Allow Javascript to read it

	session.Values[SessionIdentityKey] = email
	if err = session.Save(r, w); err != nil {
		return errors.E(op, err)
	}
	return nil
}

// AuthenticateBrowserSession updates the session for a browser, additionally
// retrieving new access tokens upon expiry. If this cannot be done, the cookie
// is deleted and an error is returned.
func (as *AuthenticationService) AuthenticateBrowserSession(ctx context.Context, w http.ResponseWriter, req *http.Request) (context.Context, error) {
	const op = errors.Op("auth.AuthenticationService.AuthenticateBrowserSession")

	session, err := as.sessionStore.Get(req, SessionName)
	if err != nil {
		return ctx, errors.E(op, err, "failed to retrieve session")
	}

	identityId, ok := session.Values[SessionIdentityKey]
	if !ok {
		return ctx, errors.E(op, "session is missing identity key")
	}

	err = as.validateAndUpdateAccessToken(ctx, identityId)

	if err != nil {
		if err := as.deleteSession(session, w, req); err != nil {
			return ctx, errors.E(op, err)
		}
		return ctx, errors.E(op, err)
	}

	ctx = contextWithSessionIdentity(ctx, identityId)

	if err := as.extendSession(session, w, req); err != nil {
		return ctx, errors.E(op, err)
	}

	return ctx, nil
}

// validateAndUpdateAccessToken validates the access tokens expiry, and if it cannot, then
// it attempts to refresh the access token.
func (as *AuthenticationService) validateAndUpdateAccessToken(ctx context.Context, email any) error {
	const op = errors.Op("auth.AuthenticationService.validateAndUpdateAccessToken")

	emailStr, ok := email.(string)
	if !ok {
		return errors.E(op, fmt.Sprintf("failed to cast email: got %T, expected %T", email, emailStr))
	}

	db := as.db
	u := &dbmodel.Identity{
		Name: emailStr,
	}
	if err := db.GetIdentity(ctx, u); err != nil {
		return errors.E(op, err)
	}

	t := &oauth2.Token{
		AccessToken:  u.AccessToken,
		RefreshToken: u.RefreshToken,
		Expiry:       u.AccessTokenExpiry,
		TokenType:    u.AccessTokenType,
	}

	// Valid simply checks the expiry, if the token isn't valid,
	// we attempt to refresh the identities tokens and update them.
	if t.Valid() {
		return nil
	}

	if err := as.refreshIdentitiesToken(ctx, emailStr, t); err != nil {
		return errors.E(op, err)
	}

	return nil
}

// refreshIdentitiesToken creates a token source based on the expired token and performs
// a manual token refresh, updating the identity afterwards.
//
// This is to be called only when a token is expired.
func (as *AuthenticationService) refreshIdentitiesToken(ctx context.Context, email string, t *oauth2.Token) error {
	const op = errors.Op("auth.AuthenticationService.refreshIdentitiesToken")

	tSrc := as.oauthConfig.TokenSource(ctx, t)

	// Get a new access and refresh token (token source only has Token())
	newToken, err := tSrc.Token()
	if err != nil {
		return errors.E(op, err, "failed to refresh token")
	}

	if err := as.UpdateIdentity(ctx, email, newToken); err != nil {
		return errors.E(op, err, "failed to update identity")
	}

	return nil
}

func (as *AuthenticationService) deleteSession(session *sessions.Session, w http.ResponseWriter, req *http.Request) error {
	const op = errors.Op("auth.AuthenticationService.deleteSession")

	if err := as.modifySession(session, w, req, -1); err != nil {
		return errors.E(op, err)
	}

	return nil
}

func (as *AuthenticationService) extendSession(session *sessions.Session, w http.ResponseWriter, req *http.Request) error {
	const op = errors.Op("auth.AuthenticationService.extendSession")

	if err := as.modifySession(session, w, req, as.sessionCookieMaxAge); err != nil {
		return errors.E(op, err)
	}

	return nil
}

func (as *AuthenticationService) modifySession(session *sessions.Session, w http.ResponseWriter, req *http.Request, maxAge int) error {
	const op = errors.Op("auth.AuthenticationService.modifySession")

	session.Options.MaxAge = maxAge

	if err := session.Save(req, w); err != nil {
		return errors.E(op, err)
	}

	return nil
}
