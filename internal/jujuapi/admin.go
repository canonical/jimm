// Copyright 2016 Canonical Ltd.

package jujuapi

import (
	"context"
	stderrors "errors"
	"sort"

	"github.com/juju/juju/rpc"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v4"

	"github.com/canonical/jimm/api/params"
	"github.com/canonical/jimm/internal/auth"
	"github.com/canonical/jimm/internal/errors"
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
// of the JWT. This in turn will be used for authentication against LoginSessionToken,
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

	// TODO(ale8k): Add vault logic to get secret key and generate one
	// on start up.
	encToken, err := authSvc.MintSessionToken(email, "secret-key")
	if err != nil {
		return response, errors.E(op, err)
	}

	response.SessionToken = string(encToken)

	return response, nil
}

// LoginSessionToken
func (r *controllerRoot) LoginSessionToken(ctx context.Context, req params.LoginSessionTokenRequest) (jujuparams.LoginResult, error) {
	const op = errors.Op("jujuapi.LoginSessionToken")
	authSvc := r.jimm.OAuthAuthenticationService()

	// TODO(ale8k): Add vault logic to get secret key and generate one
	// on start up.
	_, err := authSvc.VerifyAccessToken([]byte(req.SessionToken), "secret-key")
	if err != nil {
		var aerr *auth.AuthenticationError
		if stderrors.As(err, &aerr) {
			return aerr.LoginResult, nil
		}
		return jujuparams.LoginResult{}, errors.E(op, err)
	}

	//email := jwtToken.Subject()
	authClient := r.jimm.AuthorizationClient()

	return jujuparams.LoginResult{}, nil
}
