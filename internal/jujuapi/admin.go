// Copyright 2016 Canonical Ltd.

package jujuapi

import (
	"context"
	stderrors "errors"
	"sort"

	"github.com/juju/juju/rpc"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v4"

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
		DisplayName:      u.DisplayName,
		Identity:         u.Tag().String(),
		ControllerAccess: u.ControllerAccess,
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

// Login implements the Login method on the Admin facade.
func (r *modelRoot) Login(ctx context.Context, req jujuparams.LoginRequest) (jujuparams.LoginResult, error) {
	const op = errors.Op("jujuapi.ModelLogin")

	var err error
	r.redirectInfo, err = r.jimm.RedirectInfo(ctx, names.NewModelTag(r.uuid))
	if err != nil {
		if errors.ErrorCode(err) == errors.CodeNotFound {
			return jujuparams.LoginResult{}, errors.E(op, err, errors.CodeModelNotFound)
		}
		return jujuparams.LoginResult{}, errors.E(op, err)
	}
	// If the model was found then we'll need to redirect to it.
	servermon.LoginRedirectCount.Inc()
	return jujuparams.LoginResult{}, errors.E(op, errors.CodeRedirect, "redirection required")
}

// RedirectInfo implements the RedirectInfo method on the Admin facade.
func (r *modelRoot) RedirectInfo(ctx context.Context) (jujuparams.RedirectInfoResult, error) {
	const op = errors.Op("jujuapi.RedirectInfo")

	if r.redirectInfo.Servers == nil {
		var err error
		r.redirectInfo, err = r.jimm.RedirectInfo(ctx, names.NewModelTag(r.uuid))
		if err != nil {
			if errors.ErrorCode(err) == errors.CodeNotFound {
				return jujuparams.RedirectInfoResult{}, errors.E(op, err, errors.CodeModelNotFound)
			}
			return jujuparams.RedirectInfoResult{}, errors.E(op, err)
		}
	}
	return r.redirectInfo, nil
}
