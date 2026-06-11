// Copyright 2025 Canonical.

package login

import (
	"context"
	"database/sql"
	"net/http"

	"github.com/juju/names/v5"
	"github.com/juju/zaputil/zapctx"
	"github.com/lestrrat-go/jwx/v2/jwt"
	"go.uber.org/zap"
	"golang.org/x/oauth2"

	"github.com/canonical/jimm/v3/internal/auth"
	"github.com/canonical/jimm/v3/internal/db"
	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/logger"
	"github.com/canonical/jimm/v3/internal/openfga"
	jimmnames "github.com/canonical/jimm/v3/pkg/names"
)

// OAuthAuthenticator is responsible for handling authentication
// via OAuth2.0 AND JWT access tokens to JIMM.
type OAuthAuthenticator interface {
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
	Device(ctx context.Context) (*oauth2.DeviceAuthResponse, error)

	// DeviceAccessToken continues and collect an access token during the device login flow
	// and is step TWO.
	//
	// See Device(...) godoc for more info pertaining to the flow.
	DeviceAccessToken(ctx context.Context, res *oauth2.DeviceAuthResponse) (*oauth2.Token, error)

	// VerifyAndExtractIdentityClaims verifies the ID token in oauth2Token and
	// extracts email from the verified ID token and groups from the access token.
	VerifyAndExtractIdentityClaims(ctx context.Context, oauth2Token *oauth2.Token) (auth.IdentityClaims, error)

	// MintSessionTokenWithGroups mints a session token to be used when logging into JIMM
	// via an access token. The token contains the user's email and internal groups claim.
	MintSessionTokenWithGroups(email string, groups []string) (string, error)

	// VerifySessionToken symmetrically verifies the validty of the signature on the
	// access token JWT, returning the parsed token.
	//
	// The subject of the token contains the user's email and can be used
	// for user object creation.
	// If verification fails, return error with code CodeInvalidSessionToken
	// to indicate to the client to retry login.
	VerifySessionToken(token string) (jwt.Token, error)

	// UpdateIdentity updates the database with the display name and access token set for the user.
	// And, if present, a refresh token.
	UpdateIdentity(ctx context.Context, email string, token *oauth2.Token) error

	// VerifyClientCredentials verifies the provided client ID and client secret,
	// returning the groups claim extracted from the access token.
	VerifyClientCredentials(ctx context.Context, clientID string, clientSecret string) ([]string, error)

	// AuthenticateBrowserSession updates the session for a browser, additionally
	// retrieving new access tokens upon expiry. If this cannot be done, the cookie
	// is deleted and an error is returned.
	AuthenticateBrowserSession(ctx context.Context, w http.ResponseWriter, req *http.Request) (context.Context, error)
}

// LoginManager provides a means to manage identities within JIMM.
type LoginManager struct {
	store              *db.Database
	authSvc            *openfga.OFGAClient
	oAuthAuthenticator OAuthAuthenticator
	jimmTag            names.ControllerTag
}

// NewLoginManager returns a new loginManager that persists the roles in the provided store.
func NewLoginManager(store *db.Database, authSvc *openfga.OFGAClient, oAuthAuthenticator OAuthAuthenticator, jimmTag names.ControllerTag) (*LoginManager, error) {
	if store == nil {
		return nil, errors.New("login store cannot be nil")
	}
	if authSvc == nil {
		return nil, errors.New("login authorisation service cannot be nil")
	}
	if oAuthAuthenticator == nil {
		return nil, errors.New("oauth service cannot be nil")
	}
	if jimmTag.Id() == "" {
		return nil, errors.New("invalid jimm controller tag")
	}
	return &LoginManager{store, authSvc, oAuthAuthenticator, jimmTag}, nil
}

// LoginDevice starts the device login flow.
func (j *LoginManager) LoginDevice(ctx context.Context) (*oauth2.DeviceAuthResponse, error) {
	resp, err := j.oAuthAuthenticator.Device(ctx)

	if err != nil {
		zapctx.Error(ctx, "oauth device login failed", zap.Error(err))
		return nil, errors.Codef(errors.CodeFatalLoginError, "oauth device login failed, check JIMM's log.")
	}
	return resp, nil
}

// AuthenticateBrowserSession authenticates a browser login.
func (j *LoginManager) AuthenticateBrowserSession(ctx context.Context, w http.ResponseWriter, r *http.Request) (context.Context, error) {
	return j.oAuthAuthenticator.AuthenticateBrowserSession(ctx, w, r)
}

// GetDeviceSessionToken polls an OIDC server while a user logs in and returns a session token scoped to the user's identity.
func (j *LoginManager) GetDeviceSessionToken(ctx context.Context, deviceOAuthResponse *oauth2.DeviceAuthResponse) (string, error) {

	token, err := j.oAuthAuthenticator.DeviceAccessToken(ctx, deviceOAuthResponse)
	if err != nil {
		return "", err
	}

	claims, err := j.oAuthAuthenticator.VerifyAndExtractIdentityClaims(ctx, token)
	if err != nil {
		return "", err
	}

	if err := j.oAuthAuthenticator.UpdateIdentity(ctx, claims.Email, token); err != nil {
		return "", err
	}

	encToken, err := j.oAuthAuthenticator.MintSessionTokenWithGroups(claims.Email, claims.Groups)
	if err != nil {
		return "", err
	}

	return string(encToken), nil
}

// LoginClientCredentials verifies a user's client ID and secret before the user is logged in.
func (j *LoginManager) LoginClientCredentials(ctx context.Context, clientID string, clientSecret string) (*openfga.User, error) {

	// We expect the client to send the service account ID "as-is" and because we know that this is a clientCredentials login,
	// we can append the @serviceaccount domain to the clientID (if not already present).
	// TODO(Kian): Consider inlining the function below and removing the dependency on jimmnames.
	clientIdWithDomain, err := jimmnames.EnsureValidServiceAccountId(clientID)
	if err != nil {
		return nil, errors.Codef(errors.CodeFatalLoginError, "%w", err)
	}

	_, err = j.oAuthAuthenticator.VerifyClientCredentials(ctx, clientID, clientSecret)
	if err != nil {
		logger.LogFailedLogin(ctx, clientIdWithDomain)
		return nil, errors.Codef(errors.CodeFatalLoginError, "%w", err)
	}
	user, err := j.UserLogin(ctx, clientIdWithDomain)
	if err != nil {
		logger.LogFailedLogin(ctx, clientIdWithDomain)
		return nil, errors.Codef(errors.CodeFatalLoginError, "%w", err)
	}
	logger.LogSuccessfulLogin(ctx, clientIdWithDomain)
	return user, nil
}

// LoginWithSessionToken verifies a user's session token before the user is logged in.
func (j *LoginManager) LoginWithSessionToken(ctx context.Context, sessionToken string) (*openfga.User, error) {
	jwtToken, err := j.oAuthAuthenticator.VerifySessionToken(sessionToken)
	if err != nil {
		if errors.ErrorCode(err) == errors.CodeSessionTokenInvalid {
			logger.LogFailedLogin(ctx, "invalid session token")
			return nil, err
		}
		logger.LogFailedLogin(ctx, "unknown session token")
		return nil, errors.Codef(errors.CodeFatalLoginError, "%w", err)
	}

	email := jwtToken.Subject()
	user, err := j.UserLogin(ctx, email)
	if err != nil {
		logger.LogFailedLogin(ctx, email)
		return nil, errors.Codef(errors.CodeFatalLoginError, "%w", err)
	}
	logger.LogSuccessfulLogin(ctx, email)
	return user, nil
}

// LoginWithSessionCookie uses the identity ID expected to have come from a session cookie, to log the user in.
//
// The work to parse and store the user's identity from the session cookie takes place in internal/jimmhttp/websocket.go
// [WSHandler.ServerHTTP] during the upgrade from an HTTP connection to a websocket. The user's identity is stored
// and passed to this function with the assumption that the cookie contained a valid session. This function is far from
// the session cookie logic due to the separation between the HTTP layer and Juju's RPC mechanism.
func (j *LoginManager) LoginWithSessionCookie(ctx context.Context, identityID string) (*openfga.User, error) {

	if identityID == "" {
		return nil, errors.New("missing cookie identity")
	}
	user, err := j.UserLogin(ctx, identityID)
	if err != nil {
		logger.LogFailedLogin(ctx, identityID)
		return nil, err
	}
	logger.LogSuccessfulLogin(ctx, identityID)
	return user, nil
}

// UserLogin fetches the identity specified by a user's email or a service account ID
// and returns an openfga User that can be used to verify permissions.
// It will create a new identity if one does not exist.
// The identity's last login time is updated.
func (j *LoginManager) UserLogin(ctx context.Context, identifier string) (*openfga.User, error) {

	ofgaUser, err := j.GetOrCreateIdentity(ctx, identifier)
	if err != nil {
		return nil, errors.Codef(errors.CodeUnauthorized, "%w", err)
	}
	err = j.updateLastLogin(ctx, ofgaUser.Identity)
	if err != nil {
		return nil, err
	}
	return ofgaUser, nil
}

func (j *LoginManager) GetOrCreateIdentity(ctx context.Context, identifier string) (*openfga.User, error) {

	identity, err := dbmodel.NewIdentity(identifier)
	if err != nil {
		return nil, err
	}

	if err := j.store.GetIdentity(ctx, identity); err != nil {
		return nil, err
	}
	ofgaUser := openfga.NewUser(identity, j.authSvc)

	isJimmAdmin, err := openfga.IsAdministrator(ctx, ofgaUser, j.jimmTag)
	if err != nil {
		return nil, err
	}
	ofgaUser.JimmAdmin = isJimmAdmin

	return ofgaUser, nil
}

func (j *LoginManager) updateLastLogin(ctx context.Context, identity *dbmodel.Identity) error {
	identity.LastLogin = sql.NullTime{
		Time:  j.store.DB.NowFunc(),
		Valid: true,
	}
	return j.store.UpdateIdentity(ctx, identity)
}
