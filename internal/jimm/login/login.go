// Copyright 2025 Canonical.

package login

import (
	"context"
	"database/sql"
	"net/http"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/juju/names/v5"
	"github.com/lestrrat-go/jwx/v2/jwt"
	"golang.org/x/oauth2"

	"github.com/canonical/jimm/v3/internal/db"
	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
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

	// ExtractAndVerifyIDToken extracts the id token from the extras claims of an oauth2 token
	// and performs signature verification of the token.
	ExtractAndVerifyIDToken(ctx context.Context, oauth2Token *oauth2.Token) (*oidc.IDToken, error)

	// Email retrieves the users email from an id token via the email claim
	Email(idToken *oidc.IDToken) (string, error)

	// MintSessionToken mints a session token to be used when logging into JIMM
	// via an access token. The token only contains the user's email for authentication.
	MintSessionToken(email string) (string, error)

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

	// VerifyClientCredentials verifies the provided client ID and client secret.
	VerifyClientCredentials(ctx context.Context, clientID string, clientSecret string) error

	// AuthenticateBrowserSession updates the session for a browser, additionally
	// retrieving new access tokens upon expiry. If this cannot be done, the cookie
	// is deleted and an error is returned.
	AuthenticateBrowserSession(ctx context.Context, w http.ResponseWriter, req *http.Request) (context.Context, error)
}

// loginManager provides a means to manage identities within JIMM.
type loginManager struct {
	store              *db.Database
	authSvc            *openfga.OFGAClient
	oAuthAuthenticator OAuthAuthenticator
	jimmTag            names.ControllerTag
}

// NewLoginManager returns a new loginManager that persists the roles in the provided store.
func NewLoginManager(store *db.Database, authSvc *openfga.OFGAClient, oAuthAuthenticator OAuthAuthenticator, jimmTag names.ControllerTag) (*loginManager, error) {
	if store == nil {
		return nil, errors.E("login store cannot be nil")
	}
	if authSvc == nil {
		return nil, errors.E("login authorisation service cannot be nil")
	}
	if oAuthAuthenticator == nil {
		return nil, errors.E("oauth service cannot be nil")
	}
	return &loginManager{store, authSvc, oAuthAuthenticator, jimmTag}, nil
}

// LoginDevice starts the device login flow.
func (j *loginManager) LoginDevice(ctx context.Context) (*oauth2.DeviceAuthResponse, error) {
	const op = errors.Op("jimm.LoginDevice")
	resp, err := j.oAuthAuthenticator.Device(ctx)
	if err != nil {
		return nil, errors.E(op, err)
	}
	return resp, nil
}

// AuthenticateBrowserSession authenticates a browser login.
func (j *loginManager) AuthenticateBrowserSession(ctx context.Context, w http.ResponseWriter, r *http.Request) (context.Context, error) {
	return j.oAuthAuthenticator.AuthenticateBrowserSession(ctx, w, r)
}

// GetDeviceSessionToken polls an OIDC server while a user logs in and returns a session token scoped to the user's identity.
func (j *loginManager) GetDeviceSessionToken(ctx context.Context, deviceOAuthResponse *oauth2.DeviceAuthResponse) (string, error) {
	const op = errors.Op("jimm.GetDeviceSessionToken")

	token, err := j.oAuthAuthenticator.DeviceAccessToken(ctx, deviceOAuthResponse)
	if err != nil {
		return "", errors.E(op, err)
	}

	idToken, err := j.oAuthAuthenticator.ExtractAndVerifyIDToken(ctx, token)
	if err != nil {
		return "", errors.E(op, err)
	}

	email, err := j.oAuthAuthenticator.Email(idToken)
	if err != nil {
		return "", errors.E(op, err)
	}

	if err := j.oAuthAuthenticator.UpdateIdentity(ctx, email, token); err != nil {
		return "", errors.E(op, err)
	}

	encToken, err := j.oAuthAuthenticator.MintSessionToken(email)
	if err != nil {
		return "", errors.E(op, err)
	}

	return string(encToken), nil
}

// LoginClientCredentials verifies a user's client ID and secret before the user is logged in.
func (j *loginManager) LoginClientCredentials(ctx context.Context, clientID string, clientSecret string) (*openfga.User, error) {
	const op = errors.Op("jimm.LoginClientCredentials")
	// We expect the client to send the service account ID "as-is" and because we know that this is a clientCredentials login,
	// we can append the @serviceaccount domain to the clientID (if not already present).
	clientIdWithDomain, err := jimmnames.EnsureValidServiceAccountId(clientID)
	if err != nil {
		return nil, errors.E(op, err)
	}

	err = j.oAuthAuthenticator.VerifyClientCredentials(ctx, clientID, clientSecret)
	if err != nil {
		return nil, errors.E(op, err)
	}

	return j.UserLogin(ctx, clientIdWithDomain)
}

// LoginWithSessionToken verifies a user's session token before the user is logged in.
func (j *loginManager) LoginWithSessionToken(ctx context.Context, sessionToken string) (*openfga.User, error) {
	const op = errors.Op("jimm.LoginWithSessionToken")
	jwtToken, err := j.oAuthAuthenticator.VerifySessionToken(sessionToken)
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
func (j *loginManager) LoginWithSessionCookie(ctx context.Context, identityID string) (*openfga.User, error) {
	const op = errors.Op("jimm.LoginWithSessionCookie")
	if identityID == "" {
		return nil, errors.E(op, "missing cookie identity")
	}
	return j.UserLogin(ctx, identityID)
}

// UserLogin fetches the identity specified by a user's email or a service account ID
// and returns an openfga User that can be used to verify permissions.
// It will create a new identity if one does not exist.
// The identity's last login time is updated.
func (j *loginManager) UserLogin(ctx context.Context, identifier string) (*openfga.User, error) {
	const op = errors.Op("jimm.UpdateLastLogin")
	ofgaUser, err := j.getOrCreateIdentity(ctx, identifier)
	if err != nil {
		return nil, errors.E(op, err, errors.CodeUnauthorized)
	}
	err = j.updateLastLogin(ctx, ofgaUser.Identity)
	if err != nil {
		return nil, errors.E(op, err)
	}
	return ofgaUser, nil
}

func (j *loginManager) getOrCreateIdentity(ctx context.Context, identifier string) (*openfga.User, error) {
	const op = errors.Op("jimm.getOrCreateIdentity")

	identity, err := dbmodel.NewIdentity(identifier)
	if err != nil {
		return nil, errors.E(op, err)
	}

	if err := j.store.GetIdentity(ctx, identity); err != nil {
		return nil, err
	}
	ofgaUser := openfga.NewUser(identity, j.authSvc)

	isJimmAdmin, err := openfga.IsAdministrator(ctx, ofgaUser, j.jimmTag)
	if err != nil {
		return nil, errors.E(op, err)
	}
	ofgaUser.JimmAdmin = isJimmAdmin

	return ofgaUser, nil
}

func (j *loginManager) updateLastLogin(ctx context.Context, identity *dbmodel.Identity) error {
	identity.LastLogin = sql.NullTime{
		Time:  j.store.DB.Config.NowFunc(),
		Valid: true,
	}
	return j.store.UpdateIdentity(ctx, identity)
}
