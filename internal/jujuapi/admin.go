// Copyright 2016 Canonical Ltd.

package jujuapi

import (
	"context"
	stderrors "errors"
	"sort"
	"strings"

	"github.com/juju/juju/rpc"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v4"

	"github.com/canonical/jimm/api/params"
	"github.com/canonical/jimm/internal/auth"
	"github.com/canonical/jimm/internal/dbmodel"
	"github.com/canonical/jimm/internal/errors"
	"github.com/canonical/jimm/internal/openfga"
	"github.com/canonical/jimm/internal/servermon"
)

// unsupportedLogin returns an appropriate error for login attempts using
// old version of the Admin facade.
func unsupportedLogin() error {
	return &rpc.RequestError{
		Code:    jujuparams.CodeNotSupported,
		Message: "JIMM does not support login from old clients",
	}
}

var facadeInit = make(map[string]func(r *controllerRoot) []int)

// Login implements the Login method on the Admin facade.
func (r *controllerRoot) Login(ctx context.Context, req jujuparams.LoginRequest) (jujuparams.LoginResult, error) {
	const op = errors.Op("jujuapi.Login")

	u, err := r.jimm.Authenticate(ctx, &req)
	if err != nil {
		var aerr *auth.AuthenticationError
		if stderrors.As(err, &aerr) {
			return aerr.LoginResult, nil
		}
		return jujuparams.LoginResult{}, errors.E(op, err)
	}

	r.mu.Lock()
	r.user = u
	r.mu.Unlock()

	var facades []jujuparams.FacadeVersions
	for name, f := range facadeInit {
		facades = append(facades, jujuparams.FacadeVersions{
			Name:     name,
			Versions: f(r),
		})
	}
	sort.Slice(facades, func(i, j int) bool {
		return facades[i].Name < facades[j].Name
	})

	servermon.LoginSuccessCount.Inc()
	srvVersion, err := r.jimm.EarliestControllerVersion(ctx)
	if err != nil {
		return jujuparams.LoginResult{}, errors.E(op, err)
	}
	aui := jujuparams.AuthUserInfo{
		DisplayName: u.DisplayName,
		Identity:    u.Tag().String(),
		// TODO(Kian) CSS-6040 improve combining Postgres and OpenFGA info
		ControllerAccess: u.GetControllerAccess(ctx, r.jimm.ResourceTag()).String(),
	}
	if u.LastLogin.Valid {
		aui.LastConnection = &u.LastLogin.Time
	}
	return jujuparams.LoginResult{
		PublicDNSName: r.params.PublicDNSName,
		UserInfo:      &aui,
		ControllerTag: names.NewControllerTag(r.params.ControllerUUID).String(),
		Facades:       facades,
		ServerVersion: srvVersion.String(),
	}, nil
}

// LoginDevice starts a device login flow (typically a CLI). It will return a verification URI
// and user code that the user is expected to enter into the verification URI link.
//
// Upon successful login, the user is then expected to retrieve an access token using
// GetDeviceAccessToken.
func (r *controllerRoot) LoginDevice(ctx context.Context) (params.LoginDeviceResponse, error) {
	const op = errors.Op("jujuapi.LoginDevice")
	response := params.LoginDeviceResponse{}
	authSvc := r.jimm.OAuthAuthenticationService()

	deviceResponse, err := authSvc.Device(ctx)
	if err != nil {
		return response, errors.E(op, err)
	}

	// NOTE: As this is on the controller root struct, and a new controller root
	// is created per WS, it is EXPECTED that the subsequent call to GetDeviceSessionToken
	// happens on the SAME websocket.
	r.deviceOAuthResponse = deviceResponse

	response.VerificationURI = deviceResponse.VerificationURI
	response.UserCode = deviceResponse.UserCode

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
	authSvc := r.jimm.OAuthAuthenticationService()

	token, err := authSvc.DeviceAccessToken(ctx, r.deviceOAuthResponse)
	if err != nil {
		return response, errors.E(op, err)
	}

	idToken, err := authSvc.ExtractAndVerifyIDToken(ctx, token)
	if err != nil {
		return response, errors.E(op, err)
	}

	email, err := authSvc.Email(idToken)
	if err != nil {
		return response, errors.E(op, err)
	}

	// TODO(ale8k): Move this into a service, don't do db logic
	// at the handler level
	// Now we know who the user is, i.e., their email
	// we'll update their access token.
	// <Start of todo>
	// Build username + display name
	db := r.jimm.DB()
	u := &dbmodel.Identity{
		Name: email,
	}
	// TODO(babakks): If user does not exist, we will create one with an empty
	// display name (which we shouldn't). So it would be better to fetch
	// and then create. At the moment, GetUser is used for both create and fetch,
	// this should be changed and split apart so it is intentional what entities
	// we are creating or fetching.
	if err := db.GetIdentity(ctx, u); err != nil {
		return response, errors.E(op, err)
	}
	// Check if user has a display name, if not, set one
	if u.DisplayName == "" {
		u.DisplayName = strings.Split(email, "@")[0]
	}
	u.AccessToken = token.AccessToken
	if err := r.jimm.DB().UpdateIdentity(ctx, u); err != nil {
		return response, errors.E(op, err)
	}
	// <End of todo>

	// TODO(ale8k): Add vault logic to get secret key and generate one
	// on start up.
	encToken, err := authSvc.MintSessionToken(email, "secret-key")
	if err != nil {
		return response, errors.E(op, err)
	}

	response.SessionToken = string(encToken)

	return response, nil
}

// LoginWithSessionToken handles logging into the JIMM via a session token that JIMM has
// minted itself, this session token is simply a JWT containing the users email
// at which point the email is used to perform a lookup for the user, authorise
// whether or not they're an admin and place the user on the controller root
// such that subsequent facade method calls can access the authenticated user.
func (r *controllerRoot) LoginWithSessionToken(ctx context.Context, req params.LoginWithSessionTokenRequest) (jujuparams.LoginResult, error) {
	const op = errors.Op("jujuapi.LoginWithSessionToken")
	authenticationSvc := r.jimm.OAuthAuthenticationService()

	// Verify the session token
	jwtToken, err := authenticationSvc.VerifySessionToken(req.SessionToken, "secret-key")
	if err != nil {
		var aerr *auth.AuthenticationError
		if stderrors.As(err, &aerr) {
			return aerr.LoginResult, nil
		}
		return jujuparams.LoginResult{}, errors.E(op, err)
	}

	// Get an OpenFGA user to place on the controllerRoot for this WS
	// such that:
	//
	//	- Subsequent calls are aware of the user
	//	- Authorisation checks are done against the openfga.User
	email := jwtToken.Subject()

	// At this point, we know the user exists, so simply just get
	// the user to create the session token.
	user, err := r.jimm.GetOpenFGAUserAndAuthorise(ctx, email)
	if err != nil {
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
