// Copyright 2021 canonical.

package auth

import (
	"context"
	"fmt"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery/identchecker"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v4"

	"github.com/canonical/jimm/internal/dbmodel"
	"github.com/canonical/jimm/internal/errors"
	"github.com/canonical/jimm/internal/openfga"
	"github.com/canonical/jimm/internal/servermon"
)

// An AuthenticationError is the error returned when the requested
// authentication has failed.
type AuthenticationError struct {
	// LoginResult may contain a login challenge to send to the client.
	LoginResult jujuparams.LoginResult
}

// Error implements the error interface.
func (*AuthenticationError) Error() string {
	return "authentication failed"
}

// A JujuAuthenticator is an authenticator implementation using macaroons.
type JujuAuthenticator struct {
	Bakery           *identchecker.Bakery
	ControllerAdmins []string
	Client           *openfga.OFGAClient
}

// Authenticate implements jimm.Authenticator.
func (a JujuAuthenticator) Authenticate(ctx context.Context, req *jujuparams.LoginRequest) (*openfga.User, error) {
	const op = errors.Op("auth.Authenticate")
	if a.Client == nil {
		return nil, errors.E(op, errors.CodeServerConfiguration, "openfga client not configured")
	}
	if a.Bakery == nil {
		return nil, errors.E(op, errors.CodeServerConfiguration, "bakery not configured")
	}
	authInfo, err := a.Bakery.Checker.Auth(req.Macaroons...).Allow(ctx, identchecker.LoginOp)
	if err != nil {
		if derr, ok := err.(*bakery.DischargeRequiredError); ok {
			// Return a discharge required response.
			m, err := a.Bakery.Oven.NewMacaroon(ctx, req.BakeryVersion, derr.Caveats, derr.Ops...)
			if err != nil {
				return nil, errors.E(op, err)
			}
			return nil, &AuthenticationError{
				LoginResult: jujuparams.LoginResult{
					DischargeRequired:       m.M(),
					BakeryDischargeRequired: m,
					DischargeRequiredReason: derr.Error(),
				},
			}
		}
		servermon.AuthenticationFailCount.Inc()
		return nil, errors.E(op, err)
	}
	if !names.IsValidUser(authInfo.Identity.Id()) {
		return nil, errors.E(op, fmt.Sprintf("authenticated identity %q cannot be used as juju username", authInfo.Identity.Id()))
	}
	ut := names.NewUserTag(authInfo.Identity.Id())
	if ut.IsLocal() {
		ut = ut.WithDomain("external")
	}
	u := &dbmodel.User{
		Username:    ut.Id(),
		DisplayName: ut.Name(),
	}
	// Note: Previously here we would grant a user superuser permission if they were part of
	// a Launchpad group configurd in JIMM's config to grant superuser permission.
	// We can no longer do this in the same way with OpenFGA.
	return openfga.NewUser(u, a.Client), nil
}
