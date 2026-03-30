// Copyright 2026 Canonical.

package jujuapi

import (
	"context"
	"sort"

	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v5"
	"github.com/juju/zaputil/zapctx"
	"go.uber.org/zap"

	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/openfga"
	"github.com/canonical/jimm/v3/pkg/api/params"
)

// unsupportedLogin returns an appropriate error for login attempts using
// old version of the Admin facade.
func unsupportedLogin() error {
	return errors.Codef(errors.CodeNotSupported, "JIMM does not support login from old clients")
}

// unsupportedLoginWithInfo is a version of unsupportedLogin that logs the
// auth tag and client version for debugging purposes. This is useful for
// identifying which clients are attempting to use the old login method.
func unsupportedLoginWithInfo(ctx context.Context, req jujuparams.LoginRequest) error {
	zapctx.Debug(ctx, "unsupported login attempt",
		zap.String("auth_tag", req.AuthTag),
		zap.String("client_version", req.ClientVersion),
	)
	return unsupportedLogin()
}

var facadeInit = make(map[string]func(r *controllerRoot) []int)

// LoginDevice starts a device login flow (typically a CLI). It will return a verification URI
// and user code that the user is expected to enter into the verification URI link.
//
// Upon successful login, the user is then expected to retrieve an access token using
// GetDeviceAccessToken.
func (r *controllerRoot) LoginDevice(ctx context.Context) (params.LoginDeviceResponse, error) {

	response := params.LoginDeviceResponse{}

	deviceResponse, err := r.jimm.LoginManager().LoginDevice(ctx)
	if err != nil {
		return response, err
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

	response := params.GetDeviceSessionTokenResponse{}

	token, err := r.jimm.LoginManager().GetDeviceSessionToken(ctx, r.deviceOAuthResponse)
	if err != nil {
		return response, errors.Codef(errors.CodeUnauthorized, "%w", err)
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

	user, err := r.jimm.LoginManager().LoginWithSessionCookie(ctx, r.identityId)
	if err != nil {
		return jujuparams.LoginResult{}, errors.Codef(errors.CodeUnauthorized, "%w", err)
	}

	r.mu.Lock()
	r.user = user
	r.mu.Unlock()

	// Get server version for LoginResult
	srvVersion, err := r.jimm.JujuManager().EarliestControllerVersion(ctx)
	if err != nil {
		return jujuparams.LoginResult{}, err
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

	user, err := r.jimm.LoginManager().LoginWithSessionToken(ctx, req.SessionToken)
	if err != nil {
		// Avoid masking the error code on err below. The Juju CLI uses it to determine when to initiate login see [OAuthAuthenticator.VerifySessionToken].
		return jujuparams.LoginResult{}, err
	}

	// TODO(ale8k): This isn't needed I don't think as controller roots are unique
	// per WS, but if anyone knows different please let me know.
	r.mu.Lock()
	r.user = user
	r.mu.Unlock()

	// Get server version for LoginResult
	srvVersion, err := r.jimm.JujuManager().EarliestControllerVersion(ctx)
	if err != nil {
		return jujuparams.LoginResult{}, err
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

	user, err := r.jimm.LoginManager().LoginClientCredentials(ctx, req.ClientID, req.ClientSecret)
	if err != nil {
		return jujuparams.LoginResult{}, errors.Codef(errors.CodeUnauthorized, "%w", err)
	}

	r.mu.Lock()
	r.user = user
	r.mu.Unlock()

	// Get server version for LoginResult
	srvVersion, err := r.jimm.JujuManager().EarliestControllerVersion(ctx)
	if err != nil {
		return jujuparams.LoginResult{}, err
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

// SupportedFacades returns a map of all supported facades and their versions.
func SupportedFacades() map[string][]int {
	r := &controllerRoot{}
	supported := make(map[string][]int)
	for name, f := range facadeInit {
		supported[name] = f(r)
	}
	return supported
}
