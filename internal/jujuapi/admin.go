// Copyright 2016 Canonical Ltd.

package jujuapi

import (
	"context"
	"sort"

	jujuparams "github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/network"
	"github.com/juju/names/v4"
	"gopkg.in/errgo.v1"
	"gopkg.in/macaroon-bakery.v2/bakery"

	"github.com/CanonicalLtd/jimm/internal/auth"
	"github.com/CanonicalLtd/jimm/internal/servermon"
	"github.com/CanonicalLtd/jimm/params"
)

// admin implements the Admin facade.
type admin struct {
	root *controllerRoot
}

// Login implements the Login method on the Admin facade.
func (a admin) Login(ctx context.Context, req jujuparams.LoginRequest) (jujuparams.LoginResult, error) {
	// JIMM only supports macaroon login, ignore all the other fields.
	id, m, err := a.root.auth.Authenticate(ctx, bakery.Version1, req.Macaroons)
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
	a.root.mu.Lock()
	a.root.facades = facades
	a.root.identity = id
	a.root.mu.Unlock()

	ctx = auth.ContextWithIdentity(ctx, id)
	servermon.LoginSuccessCount.Inc()
	username := id.Id()
	srvVersion, err := a.root.jem.EarliestControllerVersion(ctx)
	if err != nil {
		return jujuparams.LoginResult{}, errgo.Mask(err)
	}
	return jujuparams.LoginResult{
		UserInfo: &jujuparams.AuthUserInfo{
			// TODO(mhilton) get a better display name from the identity manager.
			DisplayName: username,
			Identity:    userTag(username).String(),
		},
		ControllerTag: names.NewControllerTag(a.root.params.ControllerUUID).String(),
		Facades:       facadeVersions(a.root.facades),
		ServerVersion: srvVersion.String(),
	}, nil
}

// facadeVersions creates a list of facadeVersions as specified in
// facades.
func facadeVersions(facades map[facade]string) []jujuparams.FacadeVersions {
	names := make([]string, 0, len(facades))
	versions := make(map[string][]int, len(facades))
	for k := range facades {
		vs, ok := versions[k.name]
		if !ok {
			names = append(names, k.name)
		}
		versions[k.name] = append(vs, k.version)
	}
	sort.Strings(names)
	fvs := make([]jujuparams.FacadeVersions, len(names))
	for i, name := range names {
		vs := versions[name]
		sort.Ints(vs)
		fvs[i] = jujuparams.FacadeVersions{
			Name:     name,
			Versions: vs,
		}
	}
	return fvs
}

type modelAdmin struct {
	root *modelRoot
}

// Login implements the Login method on the Admin facade.
func (a modelAdmin) Login(ctx context.Context, req jujuparams.LoginRequest) (jujuparams.LoginResult, error) {
	_, _, err := a.root.modelInfo(ctx)
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
func (a modelAdmin) RedirectInfo(ctx context.Context) (jujuparams.RedirectInfoResult, error) {
	_, controller, err := a.root.modelInfo(ctx)
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
