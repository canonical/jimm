// Copyright 2016 Canonical Ltd.

package jujuapi

import (
	"context"
	"sort"

	jujuparams "github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/rpc"
	"github.com/juju/names/v4"
	"gopkg.in/errgo.v1"
	"gopkg.in/macaroon-bakery.v2/bakery"

	"github.com/CanonicalLtd/jimm/internal/auth"
	"github.com/CanonicalLtd/jimm/internal/mongodoc"
	"github.com/CanonicalLtd/jimm/internal/servermon"
	"github.com/CanonicalLtd/jimm/params"
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
	// JIMM only supports macaroon login, ignore all the other fields.
	id, m, err := r.auth.Authenticate(ctx, bakery.Version1, req.Macaroons)
	if err != nil {
		servermon.LoginFailCount.Inc()
		if m != nil {
			return jujuparams.LoginResult{
				DischargeRequired:       m.M(),
				DischargeRequiredReason: err.Error(),
			}, nil
		}
		return jujuparams.LoginResult{}, errgo.Mask(err)
	}

	r.mu.Lock()
	r.identity = id
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

	ctx = auth.ContextWithIdentity(ctx, id)
	servermon.LoginSuccessCount.Inc()
	username := id.Id()
	srvVersion, err := r.jem.EarliestControllerVersion(ctx)
	if err != nil {
		return jujuparams.LoginResult{}, errgo.Mask(err)
	}
	return jujuparams.LoginResult{
		UserInfo: &jujuparams.AuthUserInfo{
			// TODO(mhilton) get a better display name from the identity manager.
			DisplayName: username,
			Identity:    userTag(username).String(),
		},
		ControllerTag: names.NewControllerTag(r.params.ControllerUUID).String(),
		Facades:       facades,
		ServerVersion: srvVersion.String(),
	}, nil
}

// Login implements the Login method on the Admin facade.
func (r *modelRoot) Login(ctx context.Context, req jujuparams.LoginRequest) (jujuparams.LoginResult, error) {
	_, _, err := r.modelInfo(ctx)
	if err != nil {
		return jujuparams.LoginResult{}, errgo.Mask(err, errgo.Is(params.ErrModelNotFound))
	}
	// If the model was found then we'll need to redirect to it.
	servermon.LoginRedirectCount.Inc()
	return jujuparams.LoginResult{}, &jujuparams.Error{
		Code:    jujuparams.CodeRedirect,
		Message: "redirection required",
	}
}

// RedirectInfo implements the RedirectInfo method on the Admin facade.
func (r *modelRoot) RedirectInfo(ctx context.Context) (jujuparams.RedirectInfoResult, error) {
	_, controller, err := r.modelInfo(ctx)
	if err != nil {
		return jujuparams.RedirectInfoResult{}, errgo.Mask(err, errgo.Is(params.ErrModelNotFound))
	}
	servers := make([][]jujuparams.HostPort, len(controller.HostPorts))
	for i, hps := range controller.HostPorts {
		servers[i] = make([]jujuparams.HostPort, len(hps))
		for j, hp := range hps {
			servers[i][j] = jujuparams.HostPort{
				Address: jujuparams.Address{
					Value: hp.Host,
					Scope: hp.Scope,
					Type:  string(network.DeriveAddressType(hp.Host)),
				},
				Port: hp.Port,
			}
		}
	}
	return jujuparams.RedirectInfoResult{
		Servers: servers,
		CACert:  controller.CACert,
	}, nil
}

// modelInfo retrieves the data about the model.
func (r *modelRoot) modelInfo(ctx context.Context) (*mongodoc.Model, *mongodoc.Controller, error) {
	if r.model == nil {
		r.model = &mongodoc.Model{UUID: r.uuid}

		err := r.jem.DB.GetModel(ctx, r.model)
		if errgo.Cause(err) == params.ErrNotFound {
			return nil, nil, errgo.WithCausef(err, params.ErrModelNotFound, "%s", "")
		}
		if err != nil {
			return nil, nil, errgo.Mask(err)
		}
		r.controller, err = r.jem.DB.Controller(ctx, r.model.Controller)
		if err != nil {
			return nil, nil, errgo.Mask(err)
		}
	}
	return r.model, r.controller, nil
}
