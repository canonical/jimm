// Copyright 2024 Canonical.

package jujuapi

import (
	"context"
	"net/http"
	"sort"

	"github.com/juju/juju/rpc"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v5"
	"golang.org/x/oauth2"

	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/openfga"
	"github.com/canonical/jimm/v3/pkg/api/params"
)

// LoginService defines the set of methods used for login to JIMM.
type LoginService interface {
	// AuthenticateBrowserSession authenticates a session cookie is valid.
	AuthenticateBrowserSession(ctx context.Context, w http.ResponseWriter, r *http.Request) (context.Context, error)
	// LoginDevice is step 1 in the device flow and returns the OIDC server that the client should use for login.
	LoginDevice(ctx context.Context) (*oauth2.DeviceAuthResponse, error)
	// GetDeviceSessionToken polls the OIDC server waiting for the client to login and return a user scoped session token.
	GetDeviceSessionToken(ctx context.Context, deviceOAuthResponse *oauth2.DeviceAuthResponse) (string, error)
	// LoginWithClientCredentials verifies a user by their client credentials.
	LoginClientCredentials(ctx context.Context, clientID string, clientSecret string) (*openfga.User, error)
	// LoginWithSessionToken verifies a user based on their session token.
	LoginWithSessionToken(ctx context.Context, sessionToken string) (*openfga.User, error)
	// LoginWithSessionCookie verifies a user based on an identity from a cookie obtained during websocket upgrade.
	LoginWithSessionCookie(ctx context.Context, identityID string) (*openfga.User, error)
}

// unsupportedLogin returns an appropriate error for login attempts using
// old version of the Admin facade.
func unsupportedLogin() error {
	return &rpc.RequestError{
		Code:    jujuparams.CodeNotSupported,
		Message: "JIMM does not support login from old clients",
	}
}

var facadeInit = make(map[string]func(r *controllerRoot) []int)

// LoginDevice starts a device login flow (typically a CLI). It will return a verification URI
// and user code that the user is expected to enter into the verification URI link.
//
// Upon successful login, the user is then expected to retrieve an access token using
// GetDeviceAccessToken.
func (r *controllerRoot) LoginDevice(ctx context.Context) (params.LoginDeviceResponse, error) {
	const op = errors.Op("jujuapi.LoginDevice")
	response := params.LoginDeviceResponse{}

	deviceResponse, err := r.jimm.LoginDevice(ctx)
	if err != nil {
		return response, errors.E(op, err, errors.CodeUnauthorized)
	}
	// NOTE: As this is on the controller root struct, and a new controller root
	// is created per WS, it is EXPECTED that the subsequent call to GetDeviceSessionToken
	// happens on the SAME websocket.
	r.deviceOAuthResponse = deviceResponse

	response.UserCode = deviceResponse.UserCode
	response.VerificationURI = deviceResponse.VerificationURI

	return response, nil
}

// GetDeviceSessionToken retrieves an access token from the OIDC provider
// and wraps it into a JWT, using the id token's email claim for the subject
// of the JWT. This in turn will be used for authentication against LoginWithSessionToken,
// where the subject of the JWT contains the user's email - enabling identification
// of the said user's session.
func (r *controllerRoot) GetDeviceSessionToken(ctx context.Context) (params.GetDeviceSessionTokenResponse, error) {
	const op = errors.Op("jujuapi.GetDeviceSessionToken")
	response := params.GetDeviceSessionTokenResponse{}

	token, err := r.jimm.GetDeviceSessionToken(ctx, r.deviceOAuthResponse)
	if err != nil {
		return response, errors.E(op, err, errors.CodeUnauthorized)
	}

	response.SessionToken = token
	return response, nil
}

// LoginWithSessionCookie is a facade call which has the cookie intercepted at the http layer,
// in which it is then placed on the controller root under "identityId", this identityId is used
// to perform a user lookup and authorise the login call.
//
// It may be misleading in that it does not interact with cookies at all, but this will only ever
// be successful upon the http layer login being successful.
func (r *controllerRoot) LoginWithSessionCookie(ctx context.Context) (jujuparams.LoginResult, error) {
	const op = errors.Op("jujuapi.LoginWithSessionCookie")

	user, err := r.jimm.LoginWithSessionCookie(ctx, r.identityId)
	if err != nil {
		return jujuparams.LoginResult{}, errors.E(op, err, errors.CodeUnauthorized)
	}

	r.mu.Lock()
	r.user = user
	r.mu.Unlock()

	// Get server version for LoginResult
	srvVersion, err := r.jimm.EarliestControllerVersion(ctx)
	if err != nil {
		return jujuparams.LoginResult{}, errors.E(op, err)
	}

	return jujuparams.LoginResult{
		PublicDNSName: r.params.PublicDNSName,
		UserInfo:      setupAuthUserInfo(ctx, r, user),
		ControllerTag: setupControllerTag(r),
		Facades:       setupFacades(r),
		ServerVersion: srvVersion.String(),
	}, nil
}

// LoginWithSessionToken handles logging into the JIMM via a session token that JIMM has
// minted itself, this session token is simply a JWT containing the users email
// at which point the email is used to perform a lookup for the user, authorise
// whether or not they're an admin and place the user on the controller root
// such that subsequent facade method calls can access the authenticated user.
func (r *controllerRoot) LoginWithSessionToken(ctx context.Context, req params.LoginWithSessionTokenRequest) (jujuparams.LoginResult, error) {
	const op = errors.Op("jujuapi.LoginWithSessionToken")

	user, err := r.jimm.LoginWithSessionToken(ctx, req.SessionToken)
	if err != nil {
		// Avoid masking the error code on err below. The Juju CLI uses it to determine when to initiate login see [OAuthAuthenticator.VerifySessionToken].
		return jujuparams.LoginResult{}, errors.E(op, err)
	}

	// TODO(ale8k): This isn't needed I don't think as controller roots are unique
	// per WS, but if anyone knows different please let me know.
	r.mu.Lock()
	r.user = user
	r.mu.Unlock()

	// Get server version for LoginResult
	srvVersion, err := r.jimm.EarliestControllerVersion(ctx)
	if err != nil {
		return jujuparams.LoginResult{}, errors.E(op, err)
	}

	return jujuparams.LoginResult{
		PublicDNSName: r.params.PublicDNSName,
		UserInfo:      setupAuthUserInfo(ctx, r, user),
		ControllerTag: setupControllerTag(r),
		Facades:       setupFacades(r),
		ServerVersion: srvVersion.String(),
	}, nil
}

// LoginWithClientCredentials handles logging into the JIMM with the client ID
// and secret created by the IdP.
func (r *controllerRoot) LoginWithClientCredentials(ctx context.Context, req params.LoginWithClientCredentialsRequest) (jujuparams.LoginResult, error) {
	const op = errors.Op("jujuapi.LoginWithClientCredentials")

	user, err := r.jimm.LoginClientCredentials(ctx, req.ClientID, req.ClientSecret)
	if err != nil {
		return jujuparams.LoginResult{}, errors.E(err, errors.CodeUnauthorized)
	}

	r.mu.Lock()
	r.user = user
	r.mu.Unlock()

	// Get server version for LoginResult
	srvVersion, err := r.jimm.EarliestControllerVersion(ctx)
	if err != nil {
		return jujuparams.LoginResult{}, errors.E(op, err)
	}

	return jujuparams.LoginResult{
		PublicDNSName: r.params.PublicDNSName,
		UserInfo:      setupAuthUserInfo(ctx, r, user),
		ControllerTag: setupControllerTag(r),
		Facades:       setupFacades(r),
		ServerVersion: srvVersion.String(),
	}, nil
}

// setupControllerTag returns the String() of a controller tag based on the
// JIMM controller UUID.
func setupControllerTag(root *controllerRoot) string {
	return names.NewControllerTag(root.params.ControllerUUID).String()
}

// setupAuthUserInfo creates a user info object to embed into the LoginResult.
func setupAuthUserInfo(ctx context.Context, root *controllerRoot, user *openfga.User) *jujuparams.AuthUserInfo {
	aui := jujuparams.AuthUserInfo{
		DisplayName: user.DisplayName,
		Identity:    user.Tag().String(),
		// TODO(Kian) CSS-6040 improve combining Postgres and OpenFGA info
		ControllerAccess: user.GetControllerAccess(ctx, root.jimm.ResourceTag()).String(),
	}
	if user.LastLogin.Valid {
		aui.LastConnection = &user.LastLogin.Time
	}
	return &aui
}

// setupFacades ranges over all facades JIMM is aware of and sorts them into
// a versioned slice to give back to the LoginResult.
func setupFacades(root *controllerRoot) []jujuparams.FacadeVersions {
	var facades []jujuparams.FacadeVersions
	for name, f := range facadeInit {
		facades = append(facades, jujuparams.FacadeVersions{
			Name:     name,
			Versions: f(root),
		})
	}
	sort.Slice(facades, func(i, j int) bool {
		return facades[i].Name < facades[j].Name
	})
	return facades

}
