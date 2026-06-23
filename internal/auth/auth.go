// Copyright 2026 Canonical.

// Package auth provides means to authenticate users into JIMM.
//
// The methods of authentication are:
// - OAuth2.0 (Device flow)
// - OAuth2.0 (Browser flow)
// - JWTs (For CLI based sessions)
package auth

import (
	"context"
	"crypto/rand"
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

	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/servermon"
	"github.com/canonical/jimm/v3/pkg/api/params"
)

const (
	// SessionName is the name of the gorilla session and is used to retrieve
	// the session object from the database.
	SessionName = "jimm-browser-session"

	// SessionIdentityKey is the key for the identity value stored within the
	// session.
	SessionIdentityKey = "identity-id"

	// SessionGroupsKey is the key for the groups value stored within the session.
	SessionGroupsKey = "identity-groups"

	// StateKey is the key for the OAuth callback state stored within a user's cookie.
	StateKey = "jimm-oauth-state"

	// SessionTokenGroupsClaimKey is the stable internal claim used by JIMM when
	// minting session tokens that carry group identifiers.
	SessionTokenGroupsClaimKey = "groups"

	// migrationTokenExpiry is the expiry time for migration tokens.
	migrationTokenExpiry = 3 * time.Hour
	idpGroupLimit        = 20
)

// AuthStyle determines how the client credentials are sent to the token endpoint.
//
// This is relevant because some identity providers expect the client credentials
// to be sent in the request body, while others expect them to be sent in the
// request header. The x/oauth2 package defaults to auto-detect this by trying
// one method and falling back to the other if it gets an error. This leads to
// rate-limit errors from the provider and the device flow ends up taking much
// longer than necessary to complete.
type AuthStyle string

const (
	// OAuthStyleInParams indicates that the client credentials should be sent in the request body.
	OAuthStyleInParams AuthStyle = "in_params"
	// OAuthStyleInHeader indicates that the client credentials should be sent in the request header.
	OAuthStyleInHeader AuthStyle = "in_header"
	// oAuthStyleAuto indicates that the client should automatically detect how to send the client credentials.
	OAuthStyleAuto AuthStyle = "auto"
)

type sessionIdentityContextKey struct{}
type sessionGroupsContextKey struct{}

// ContextWithSessionIdentity adds the session identity id to the provided context.
func ContextWithSessionIdentity(ctx context.Context, sessionIdentityId any) context.Context {
	return context.WithValue(ctx, sessionIdentityContextKey{}, sessionIdentityId)
}

// SessionIdentityFromContext returns the session identity key from the context.
func SessionIdentityFromContext(ctx context.Context) string {
	v := ctx.Value(sessionIdentityContextKey{})
	if v == nil {
		return ""
	}
	s, ok := v.(string)
	if !ok {
		zapctx.Error(ctx, "failed to retrieve identity string from context", zap.Any("identity", v))
		return ""
	}
	return s
}

// SessionGroupsFromContext returns the session groups from the context.
func SessionGroupsFromContext(ctx context.Context) []string {
	groups, _ := ctx.Value(sessionGroupsContextKey{}).([]string)
	return append([]string(nil), groups...)
}

// ContextWithSessionGroups adds the session groups to the provided context.
func ContextWithSessionGroups(ctx context.Context, groups []string) context.Context {
	return context.WithValue(ctx, sessionGroupsContextKey{}, append([]string(nil), groups...))
}

func sessionGroupsFromValue(value any) ([]string, error) {
	if value == nil {
		return nil, nil
	}
	if groups, ok := value.([]string); ok {
		return append([]string(nil), groups...), nil
	}
	if groups, ok := value.([]any); ok {
		parsedGroups := make([]string, 0, len(groups))
		for i, group := range groups {
			groupStr, ok := group.(string)
			if !ok {
				return nil, fmt.Errorf("invalid session group entry type at index %d: got %T", i, group)
			}
			parsedGroups = append(parsedGroups, groupStr)
		}
		return parsedGroups, nil
	}
	return nil, fmt.Errorf("invalid session groups type: got %T", value)
}

// AuthenticationService handles authentication within JIMM.
type AuthenticationService struct {
	oauthConfig oauth2.Config
	// clientCredentialScopes holds scopes used only for client-credentials flow.
	clientCredentialScopes []string
	// provider holds a OIDC provider wrapper for the OAuth2.0 /x/oauth package,
	// enabling UserInfo calls, wellknown retrieval and jwks verification.
	provider *oidc.Provider
	// sessionTokenExpiry holds the expiry time for JIMM minted session tokens (JWTs).
	sessionTokenExpiry time.Duration
	// sessionCookieMaxAge holds the max age for session cookies in seconds.
	sessionCookieMaxAge int
	// secureCookies decides whether to set the secure flag on cookies.
	secureCookies bool
	// jwtSessionKey holds the secret key used for signing/verifying JWT tokens.
	// According to https://datatracker.ietf.org/doc/html/rfc7518 minimum key lengths are
	// HSXXX e.g. HS256 - 256 bits, RSA - at least 2048 bits.
	// In JIMM we use HS256, requiring a minimum of 32 bytes for the secret key.
	jwtSessionKey string
	// The key algorithm to use for verifying/signing JWTs.
	signingAlg jwa.KeyAlgorithm
	// groupClaimKey is the provider-specific claim name that contains the user's
	// group identifiers.
	groupClaimKey string

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

	// Scopes holds scopes requested for browser/device OAuth flows.
	Scopes []string

	// ClientCredentialScopes holds scopes requested for client-credentials flow.
	ClientCredentialScopes []string

	// GroupClaimKey is the provider-specific claim name that contains group
	// identifiers.
	GroupClaimKey string

	// SessionTokenExpiry holds the expiry time of minted JIMM session tokens (JWTs).
	SessionTokenExpiry time.Duration

	// SessionCookieMaxAge holds the max age for session cookies in seconds.
	SessionCookieMaxAge int

	// SecureCookies decides whether to set the secure flag on cookies.
	SecureCookies bool

	// JWTSessionKey holds the secret key used for signing/verifying JWT tokens.
	// See AuthenticationService.JWTSessionKey for more details.
	JWTSessionKey string

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

	// AuthStyle configures how the client credentials should be sent to the token endpoint.
	AuthStyle AuthStyle
}

// IdentityClaims are the user identity values extracted from verified OIDC tokens.
type IdentityClaims struct {
	Subject string
	Email   string
	Groups  []string
}

// NewAuthenticationService returns a new authentication service for handling
// authentication within JIMM.
func NewAuthenticationService(ctx context.Context, params AuthenticationServiceParams) (*AuthenticationService, error) {
	provider, err := oidc.NewProvider(ctx, params.IssuerURL)
	if err != nil {
		return nil, errors.Codef(errors.CodeServerConfiguration, "failed to create oidc provider: %v", err)
	}

	authSvc := &AuthenticationService{
		provider: provider,
		oauthConfig: oauth2.Config{
			ClientID:     params.ClientID,
			ClientSecret: params.ClientSecret,
			Endpoint:     provider.Endpoint(),
			Scopes:       params.Scopes,
			RedirectURL:  params.RedirectURL,
		},
		clientCredentialScopes: params.ClientCredentialScopes,
		sessionTokenExpiry:     params.SessionTokenExpiry,
		jwtSessionKey:          params.JWTSessionKey,
		signingAlg:             jwa.HS256,
		groupClaimKey:          params.GroupClaimKey,
		db:                     params.Store,
		sessionStore:           params.SessionStore,
		sessionCookieMaxAge:    params.SessionCookieMaxAge,
		secureCookies:          params.SecureCookies,
	}

	// If the auth style is specifically defined, then use that to avoid
	// the pit-falls of auto-detection (sending 2 requests each time).
	// See the docstring for the AuthStyle type for more information.
	switch params.AuthStyle {
	case OAuthStyleInHeader:
		authSvc.oauthConfig.Endpoint.AuthStyle = oauth2.AuthStyleInHeader
	case OAuthStyleInParams:
		authSvc.oauthConfig.Endpoint.AuthStyle = oauth2.AuthStyleInParams
	default:
		authSvc.oauthConfig.Endpoint.AuthStyle = oauth2.AuthStyleAutoDetect
	}

	return authSvc, nil
}

// splitGroupClaimString normalises a string claim into groups.
//
// Supported formats:
// - single value: "team-a"
// - comma delimited: "team-a, team-b"
// - whitespace delimited: "team-a team-b"
func splitGroupClaimString(value string) []string {
	trimmedValue := strings.TrimSpace(value)
	if trimmedValue == "" {
		return nil
	}

	normalised := strings.ReplaceAll(trimmedValue, ",", " ")
	return strings.Fields(normalised)
}

// extractGroupsFromAccessToken extracts the configured groups claim from an access token.
// The access token is parsed as a JWT without signature verification because it
// originates from the provider token endpoint and we only need claim extraction.
func (as *AuthenticationService) extractGroupsFromAccessToken(ctx context.Context, accessToken *oauth2.Token) ([]string, error) {
	if accessToken == nil || accessToken.AccessToken == "" {
		return nil, errors.New("access token is empty")
	}

	if as.groupClaimKey == "" {
		return nil, nil
	}

	parsedToken, err := jwt.ParseInsecure([]byte(accessToken.AccessToken))
	if err != nil {
		return nil, fmt.Errorf("failed to parse access token: %v", err)
	}

	groupClaim, ok := parsedToken.Get(as.groupClaimKey)
	if !ok {
		zapctx.Warn(ctx, "configured group claim missing from access token", zap.String("claim-key", as.groupClaimKey))
		return nil, nil
	}

	var groups []string

	// Normalize the group claim into a slice of strings.
	switch typedGroups := groupClaim.(type) {
	case string:
		groups = splitGroupClaimString(typedGroups)
	case []string:
		groups = typedGroups
	case []any:
		groups = make([]string, 0, len(typedGroups))
		for i, group := range typedGroups {
			groupStr, ok := group.(string)
			if !ok {
				return nil, fmt.Errorf("invalid group claim entry type at index %d: got %T", i, group)
			}
			groups = append(groups, groupStr)
		}
	default:
		return nil, fmt.Errorf("invalid group claim type: got %T", groupClaim)
	}

	if len(groups) > idpGroupLimit {
		return nil, errors.Codef(errors.CodeUnauthorized, "authorization denied: IDP group claim contains %d groups, maximum supported is %d", len(groups), idpGroupLimit)
	}

	return groups, nil
}

// AuthCodeURL returns a URL that will be used to redirect a browser to the identity provider.
// It also generates a random state string that was used as part of the auth code URL. The state string
// is returned alongside the auth code URL and any errors that occured during state generation.
func (as *AuthenticationService) AuthCodeURL() (string, string, error) {
	// Hydra requires the state parameter to be at least 8 characters.
	// Note that state is primarily a guard against csrf attacks.
	// A good reference is https://spring.io/blog/2011/11/30/cross-site-request-forgery-and-oauth2
	// Because Hydra only accepts return addresses that have been pre-registered
	// the risk of csrf attacks is largely eliminated, but this may not be the case with other IdPs.

	b := make([]byte, 8)
	_, err := rand.Read(b)
	if err != nil {
		return "", "", fmt.Errorf("failed to generate state secret: %s", err.Error())
	}
	state := base64.RawURLEncoding.EncodeToString(b)
	return as.oauthConfig.AuthCodeURL(state), state, nil
}

// Exchange exchanges an authorisation code for an access token.
//
// TODO(ale8k): How to test this? A callback has to be made and it needs to be valid,
// this may need some thought as to whether its actually worth testing or are we
// just testing the library. The handler test essentially covers this so perhaps
// its ok to leave it as is?
func (as *AuthenticationService) Exchange(ctx context.Context, code string) (*oauth2.Token, error) {

	t, err := as.oauthConfig.Exchange(
		ctx,
		code,
		oauth2.SetAuthURLParam("client_secret", as.oauthConfig.ClientSecret),
	)
	if err != nil {
		return nil, fmt.Errorf("authorisation code exchange failed: %v", err)
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

	resp, err := as.oauthConfig.DeviceAuth(
		ctx,
		oauth2.SetAuthURLParam("client_secret", as.oauthConfig.ClientSecret),
	)
	if err != nil {
		return nil, fmt.Errorf("device auth call failed: %v", err)
	}

	return resp, nil
}

// DeviceAccessToken continues and collect an access token during the device login flow
// and is step TWO.
//
// See Device(...) godoc for more info pertaining to the flow.
func (as *AuthenticationService) DeviceAccessToken(ctx context.Context, res *oauth2.DeviceAuthResponse) (*oauth2.Token, error) {

	t, err := as.oauthConfig.DeviceAccessToken(
		ctx,
		res,
		oauth2.SetAuthURLParam("client_secret", as.oauthConfig.ClientSecret),
	)
	if err != nil {
		return nil, fmt.Errorf("device access token call failed: %v", err)
	}

	return t, nil
}

// VerifyAndExtractIdentityClaims verifies the ID token inside oauth2Token and
// returns identity claims where email comes from the verified ID token and
// groups come from the access token.
func (as *AuthenticationService) VerifyAndExtractIdentityClaims(ctx context.Context, oauth2Token *oauth2.Token) (IdentityClaims, error) {
	// Extract the ID Token from oauth2 token.
	rawIDToken, ok := oauth2Token.Extra("id_token").(string)
	if !ok {
		return IdentityClaims{}, errors.New("failed to extract id token")
	}

	verifier := as.provider.Verifier(&oidc.Config{
		ClientID: as.oauthConfig.ClientID,
	})

	idToken, err := verifier.Verify(ctx, rawIDToken)
	if err != nil {
		return IdentityClaims{}, fmt.Errorf("failed to verify id token: %v", err)
	}

	claims, err := as.extractClaims(ctx, idToken, oauth2Token)
	if err != nil {
		return IdentityClaims{}, err
	}

	return claims, nil
}

// extractClaims extracts the email from a verified ID token and the configured
// groups claim from the access token.
func (as *AuthenticationService) extractClaims(ctx context.Context, idToken *oidc.IDToken, accessToken *oauth2.Token) (IdentityClaims, error) {
	if idToken == nil {
		return IdentityClaims{}, errors.New("id token is nil")
	}

	emailClaimStr, err := as.extractEmailClaim(idToken)
	if err != nil {
		return IdentityClaims{}, err
	}

	claims := IdentityClaims{Subject: idToken.Subject, Email: emailClaimStr}

	// Extract groups from the access token
	groups, err := as.extractGroupsFromAccessToken(ctx, accessToken)
	if err != nil {
		return IdentityClaims{}, fmt.Errorf("failed to extract groups from access token: %v", err)
	}
	claims.Groups = groups

	return claims, nil
}

// extractEmailClaim extracts the email claim from the verified ID token.
func (as *AuthenticationService) extractEmailClaim(idToken *oidc.IDToken) (string, error) {
	if idToken == nil {
		return "", errors.New("id token is nil")
	}

	var rawClaims map[string]any
	if err := idToken.Claims(&rawClaims); err != nil {
		return "", fmt.Errorf("failed to extract claims: %v", err)
	}

	emailClaim, ok := rawClaims["email"]
	if !ok {
		return "", errors.New("missing email claim")
	}

	emailClaimStr, ok := emailClaim.(string)
	if !ok {
		return "", fmt.Errorf("email claim is not a string: got %T", emailClaim)
	}

	return emailClaimStr, nil
}

// MintSessionTokenWithGroups mints a session token that carries the user's
// email and the internal groups claim for later authorization use.
func (as *AuthenticationService) MintSessionTokenWithGroups(email string, groups []string) (string, error) {

	builder := jwt.NewBuilder().
		Subject(email).
		Expiration(time.Now().Add(as.sessionTokenExpiry))
	if len(groups) > 0 {
		builder = builder.Claim(SessionTokenGroupsClaimKey, groups)
	}

	token, err := builder.Build()
	if err != nil {
		return "", fmt.Errorf("failed to build access token: %v", err)
	}

	freshToken, err := jwt.Sign(token, jwt.WithKey(as.signingAlg, []byte(as.jwtSessionKey)))
	if err != nil {
		return "", fmt.Errorf("failed to sign access token: %v", err)
	}

	return base64.StdEncoding.EncodeToString(freshToken), nil
}

// NewMigrationToken mints a migration token for a Juju controller to use when
// migrating a model to JAAS. The token is used by a Juju controller to login
// on the user's behalf and migrate the model.
//
// The token carries the user's groups so that the controller can verify group
// membership. It keeps the same structure as session tokens for consistency.
func (as *AuthenticationService) NewMigrationToken(ctx context.Context, username string, groups []string) (string, error) {

	builder := jwt.NewBuilder().
		Subject(username).
		Expiration(time.Now().Add(migrationTokenExpiry))

	if len(groups) > 0 {
		builder = builder.Claim(SessionTokenGroupsClaimKey, groups)
	}

	token, err := builder.Build()
	if err != nil {
		return "", fmt.Errorf("failed to mint migration token: %v", err)
	}

	migrationToken, err := jwt.Sign(token, jwt.WithKey(as.signingAlg, []byte(as.jwtSessionKey)))
	if err != nil {
		return "", fmt.Errorf("failed to sign migration token: %v", err)
	}

	return base64.StdEncoding.EncodeToString(migrationToken), nil
}

// VerifySessionToken symmetrically verifies the validty of the signature on the
// access token JWT, returning the parsed token.
//
// The subject of the token contains the user's email and can be used
// for user object creation.
//
// The error code returned here is used by the Juju CLI to know when to start a
// device login flow, prompting the user to login again.
func (as *AuthenticationService) VerifySessionToken(token string) (_ jwt.Token, err error) {
	defer func() {
		if err != nil {
			servermon.AuthenticationFailCount.WithLabelValues("VerifySessionToken").Inc()
		} else {
			servermon.AuthenticationSuccessCount.WithLabelValues("VerifySessionToken").Inc()
		}
	}()

	if len(token) == 0 {
		return nil, errors.Codef(errors.CodeSessionTokenInvalid, "no token presented")
	}

	decodedToken, err := base64.StdEncoding.DecodeString(token)
	if err != nil {
		return nil, fmt.Errorf("failed to decode token: %w", err)
	}

	parsedToken, err := jwt.Parse(decodedToken, jwt.WithKey(as.signingAlg, []byte(as.jwtSessionKey)))
	if err != nil {
		if stderrors.Is(err, jwt.ErrTokenExpired()) {
			return nil, errors.Codef(errors.CodeSessionTokenInvalid, "JIMM session token expired")
		}
		return nil, err
	}

	if _, err = mail.ParseAddress(parsedToken.Subject()); err != nil {
		return nil, errors.Codef(errors.CodeSessionTokenInvalid, "failed to parse email")
	}

	return parsedToken, nil
}

// SessionGroupsFromToken extracts the stable internal groups claim from a
// verified JIMM session token.
func SessionGroupsFromToken(token jwt.Token) ([]string, error) {
	if token == nil {
		return nil, errors.New("token is nil")
	}

	rawGroups, ok := token.Get(SessionTokenGroupsClaimKey)
	if !ok {
		return nil, nil
	}

	if parsedGroups, ok := rawGroups.([]any); ok {
		var groups []string
		for i, group := range parsedGroups {
			groupStr, ok := group.(string)
			if !ok {
				return nil, fmt.Errorf("invalid group claim entry type at index %d: got %T", i, group)
			}
			groups = append(groups, groupStr)
		}
		return groups, nil
	}

	return nil, fmt.Errorf("invalid %q claim type %T", SessionTokenGroupsClaimKey, rawGroups)
}

// UpdateIdentity updates the database with the display name and access token set for the user.
// And, if present, a refresh token.
func (as *AuthenticationService) UpdateIdentity(ctx context.Context, email string, token *oauth2.Token) error {

	db := as.db

	// TODO(ale8k): Add test case for this
	u, err := dbmodel.NewIdentity(email)
	if err != nil {
		return err
	}

	// TODO(babakks): If user does not exist, we will create one with an empty
	// display name (which we shouldn't). So it would be better to fetch
	// and then create. At the moment, GetUser is used for both create and fetch,
	// this should be changed and split apart so it is intentional what entities
	// we are creating or fetching.
	if err := db.GetIdentity(ctx, u); err != nil {
		return err
	}

	u.AccessToken = token.AccessToken
	u.RefreshToken = token.RefreshToken
	u.AccessTokenExpiry = token.Expiry
	u.AccessTokenType = token.TokenType
	if err := db.UpdateIdentity(ctx, u); err != nil {
		return err
	}

	return nil
}

// VerifyClientCredentials verifies the provided client ID and client secret,
// and extracts the groups claim from the returned access token.
func (as *AuthenticationService) VerifyClientCredentials(ctx context.Context, clientID string, clientSecret string) ([]string, error) {
	cfg := clientcredentials.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		TokenURL:     as.oauthConfig.Endpoint.TokenURL,
		AuthStyle:    oauth2.AuthStyle(as.oauthConfig.Endpoint.AuthStyle),
		Scopes:       as.clientCredentialScopes,
	}

	accessToken, err := cfg.Token(ctx)
	if err != nil {
		servermon.AuthenticationFailCount.WithLabelValues("VerifyClientCredentials").Inc()
		return nil, errors.Codef(errors.CodeUnauthorized, "invalid client credentials: %v", err)
	}

	// Extract groups from the access token
	groups, err := as.extractGroupsFromAccessToken(ctx, accessToken)
	if err != nil {
		servermon.AuthenticationFailCount.WithLabelValues("VerifyClientCredentials").Inc()
		return nil, fmt.Errorf("failed to extract groups from access token: %v", err)
	}

	servermon.AuthenticationSuccessCount.WithLabelValues("VerifyClientCredentials").Inc()
	return groups, nil
}

// sessionCrossOriginSafe sets parameters on the session that allow its use in cross-origin requests.
// Options are not saved to the database so this must be called whenever a session cookie will be returned to a client.
//
// Note browsers require cookies with the same-site policy as 'none' to additionally have the secure flag set.
func sessionCrossOriginSafe(session *sessions.Session, secure bool) *sessions.Session {
	session.Options.Secure = secure                  // Ensures only sent with HTTPS
	session.Options.HttpOnly = true                  // Don't allow Javascript to modify cookie
	session.Options.SameSite = http.SameSiteNoneMode // Allow cross-origin requests via Javascript
	return session
}

// CreateBrowserSessionWithGroups creates a browser session that stores the
// authenticated identity and the extracted group identifiers.
func (as *AuthenticationService) CreateBrowserSessionWithGroups(
	ctx context.Context,
	w http.ResponseWriter,
	r *http.Request,
	email string,
	groups []string,
) error {

	session, err := as.sessionStore.Get(r, SessionName)
	if err != nil {
		return err
	}

	session.IsNew = true                            // Sets cookie to a fresh new cookie
	session.Options.MaxAge = as.sessionCookieMaxAge // Expiry in seconds
	session = sessionCrossOriginSafe(session, as.secureCookies)

	session.Values[SessionIdentityKey] = email
	session.Values[SessionGroupsKey] = groups
	if err = session.Save(r, w); err != nil {
		return err
	}
	return nil
}

// AuthenticateBrowserSession updates the session for a browser, additionally
// retrieving new access tokens upon expiry. If this cannot be done, the cookie
// is deleted and an error is returned.
func (as *AuthenticationService) AuthenticateBrowserSession(ctx context.Context, w http.ResponseWriter, req *http.Request) (_ context.Context, err error) {

	defer func() {
		if err != nil {
			servermon.AuthenticationFailCount.WithLabelValues("AuthenticateBrowserSession").Inc()
		} else {
			servermon.AuthenticationSuccessCount.WithLabelValues("AuthenticateBrowserSession").Inc()
		}
	}()

	session, err := as.sessionStore.Get(req, SessionName)
	if err != nil {
		return ctx, fmt.Errorf("failed to retrieve session: %v", err)
	}
	session = sessionCrossOriginSafe(session, as.secureCookies)

	identityId, ok := session.Values[SessionIdentityKey]
	if !ok {
		return ctx, errors.Codef(errors.CodeForbidden, "session is missing identity key")
	}

	err = as.validateAndUpdateAccessToken(ctx, identityId)
	if err != nil {
		// If the user's access token AND refresh token have expired
		// then we will fail authentication here.
		return ctx, fmt.Errorf("failed to validate and update status token: %v", err)
	}

	ctx = ContextWithSessionIdentity(ctx, identityId)
	groups, err := sessionGroupsFromValue(session.Values[SessionGroupsKey])
	if err != nil {
		return ctx, err
	}
	ctx = ContextWithSessionGroups(ctx, groups)

	if err := as.extendSession(session, w, req); err != nil {
		return ctx, err
	}

	return ctx, nil
}

// Logout does two things:
//
//   - It deletes the session (Max-Age = -1), and within the database the cleanup routine will remove
//     the expired session upon next run.
//   - It resets the access tokens for this user
func (as *AuthenticationService) Logout(ctx context.Context, w http.ResponseWriter, req *http.Request) error {

	session, err := as.sessionStore.Get(req, SessionName)
	if err != nil {
		return fmt.Errorf("failed to retrieve session: %v", err)
	}

	identityId, ok := session.Values[SessionIdentityKey]
	if !ok {
		return errors.New("session is missing identity key")
	}

	identityIdStr, ok := identityId.(string)
	if !ok {
		return fmt.Errorf("session identity key could not be parsed: expected %T, got %T", identityIdStr, identityId)
	}

	if err := as.deleteSession(session, w, req); err != nil {
		return fmt.Errorf("failed to delete session: %v", err)
	}

	if err := as.UpdateIdentity(ctx, identityIdStr, &oauth2.Token{
		AccessToken:  "",
		RefreshToken: "",
		Expiry:       time.Now(),
		TokenType:    "",
	}); err != nil {
		return fmt.Errorf("failed to update identity: %v", err)
	}

	return nil
}

// Whoami returns "whoami" response, based on the identity id populating the fields
// according to the current database schema for identities. This is likely subject
// to change in the future.
func (as *AuthenticationService) Whoami(ctx context.Context) (*params.WhoamiResponse, error) {

	identityId := SessionIdentityFromContext(ctx)
	if identityId == "" {
		return nil, errors.New("no identity in context")
	}

	// TODO(ale8k) CSS-8227: Add test case for this
	u, err := dbmodel.NewIdentity(identityId)
	if err != nil {
		return nil, err
	}

	if err := as.db.GetIdentity(ctx, u); err != nil {
		return nil, err
	}

	return &params.WhoamiResponse{
		DisplayName: u.DisplayName,
		Email:       u.Name,
	}, nil

}

// validateAndUpdateAccessToken validates the access tokens expiry, and if it cannot, then
// it attempts to refresh the access token.
func (as *AuthenticationService) validateAndUpdateAccessToken(ctx context.Context, email any) error {

	emailStr, ok := email.(string)
	if !ok {
		return fmt.Errorf("failed to cast email: got %T, expected %T", email, emailStr)
	}

	db := as.db

	// TODO(ale8k) CSS-8228: Add test case for this
	u, err := dbmodel.NewIdentity(emailStr)
	if err != nil {
		return err
	}

	if err := db.GetIdentity(ctx, u); err != nil {
		return err
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
		return err
	}

	return nil
}

// refreshIdentitiesToken creates a token source based on the expired token and performs
// a manual token refresh, updating the identity afterwards.
//
// This is to be called only when a token is expired.
func (as *AuthenticationService) refreshIdentitiesToken(ctx context.Context, email string, t *oauth2.Token) error {

	tSrc := as.oauthConfig.TokenSource(ctx, t)

	// Get a new access and refresh token (token source only has Token())
	newToken, err := tSrc.Token()
	if err != nil {
		return fmt.Errorf("failed to refresh token: %w", err)
	}

	if err := as.UpdateIdentity(ctx, email, newToken); err != nil {
		return fmt.Errorf("failed to update identity: %w", err)
	}

	return nil
}

func (as *AuthenticationService) deleteSession(session *sessions.Session, w http.ResponseWriter, req *http.Request) error {

	if err := as.modifySession(session, w, req, -1); err != nil {
		return err
	}

	return nil
}

func (as *AuthenticationService) extendSession(session *sessions.Session, w http.ResponseWriter, req *http.Request) error {

	if err := as.modifySession(session, w, req, as.sessionCookieMaxAge); err != nil {
		return err
	}

	return nil
}

func (as *AuthenticationService) modifySession(session *sessions.Session, w http.ResponseWriter, req *http.Request, maxAge int) error {

	session.Options.MaxAge = maxAge

	if err := session.Save(req, w); err != nil {
		return err
	}

	return nil
}
