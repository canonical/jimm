// Copyright 2021 CanonicalLtd.

package auth_test

import (
	"context"
	"database/sql"
	"testing"

	qt "github.com/frankban/quicktest"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery/checkers"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery/identchecker"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakerytest"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/httpbakery"
	jujuparams "github.com/juju/juju/rpc/params"
	"gopkg.in/macaroon.v2"

	"github.com/CanonicalLtd/jimm/internal/auth"
	"github.com/CanonicalLtd/jimm/internal/dbmodel"
	"github.com/CanonicalLtd/jimm/internal/errors"
)

func TestAuthenticateLogin(t *testing.T) {
	c := qt.New(t)

	discharger := bakerytest.NewDischarger(nil)
	c.Cleanup(discharger.Close)
	discharger.CheckerP = httpbakery.ThirdPartyCaveatCheckerPFunc(
		func(ctx context.Context, p httpbakery.ThirdPartyCaveatCheckerParams) ([]checkers.Caveat, error) {
			return []checkers.Caveat{checkers.DeclaredCaveat("username", "alice")}, nil
		},
	)
	authenticator := auth.JujuAuthenticator{
		Bakery: identchecker.NewBakery(identchecker.BakeryParams{
			Locator:        discharger,
			Key:            bakery.MustGenerateKey(),
			IdentityClient: testIdentityClient{loc: discharger.Location()},
			Location:       "jimm",
			Logger:         testLogger{t: c},
		}),
	}

	ctx := context.Background()
	u, err := authenticator.Authenticate(ctx, &jujuparams.LoginRequest{})
	c.Check(u, qt.IsNil)
	aerr, ok := err.(*auth.AuthenticationError)
	c.Assert(ok, qt.Equals, true, qt.Commentf("unexpected error %s", err))

	client := httpbakery.NewClient()
	ms, err := client.DischargeAll(ctx, aerr.LoginResult.BakeryDischargeRequired)
	c.Assert(err, qt.IsNil)
	u, err = authenticator.Authenticate(ctx, &jujuparams.LoginRequest{Macaroons: []macaroon.Slice{ms}})
	c.Assert(err, qt.IsNil)
	c.Check(u.LastLogin.Valid, qt.Equals, false)
	u.LastLogin = sql.NullTime{}
	c.Check(u, qt.DeepEquals, &dbmodel.User{
		Username:    "alice@external",
		DisplayName: "alice",
	})
}

func TestAuthenticateLoginWithDomain(t *testing.T) {
	c := qt.New(t)

	discharger := bakerytest.NewDischarger(nil)
	c.Cleanup(discharger.Close)
	discharger.CheckerP = httpbakery.ThirdPartyCaveatCheckerPFunc(
		func(ctx context.Context, p httpbakery.ThirdPartyCaveatCheckerParams) ([]checkers.Caveat, error) {
			return []checkers.Caveat{checkers.DeclaredCaveat("username", "alice@mydomain")}, nil
		},
	)
	authenticator := auth.JujuAuthenticator{
		Bakery: identchecker.NewBakery(identchecker.BakeryParams{
			Locator:        discharger,
			Key:            bakery.MustGenerateKey(),
			IdentityClient: testIdentityClient{loc: discharger.Location()},
			Location:       "jimm",
			Logger:         testLogger{t: c},
		}),
	}

	ctx := context.Background()
	u, err := authenticator.Authenticate(ctx, &jujuparams.LoginRequest{})
	c.Check(u, qt.IsNil)
	aerr, ok := err.(*auth.AuthenticationError)
	c.Assert(ok, qt.Equals, true, qt.Commentf("unexpected error %s", err))

	client := httpbakery.NewClient()
	ms, err := client.DischargeAll(ctx, aerr.LoginResult.BakeryDischargeRequired)
	c.Assert(err, qt.IsNil)
	u, err = authenticator.Authenticate(ctx, &jujuparams.LoginRequest{Macaroons: []macaroon.Slice{ms}})
	c.Assert(err, qt.IsNil)
	c.Check(u.LastLogin.Valid, qt.Equals, false)
	u.LastLogin = sql.NullTime{}
	c.Check(u, qt.DeepEquals, &dbmodel.User{
		Username:    "alice@mydomain",
		DisplayName: "alice",
	})
}

func TestAuthenticateLoginSuperuser(t *testing.T) {
	c := qt.New(t)

	discharger := bakerytest.NewDischarger(nil)
	c.Cleanup(discharger.Close)
	discharger.CheckerP = httpbakery.ThirdPartyCaveatCheckerPFunc(
		func(ctx context.Context, p httpbakery.ThirdPartyCaveatCheckerParams) ([]checkers.Caveat, error) {
			return []checkers.Caveat{checkers.DeclaredCaveat("username", "bob")}, nil
		},
	)
	authenticator := auth.JujuAuthenticator{
		Bakery: identchecker.NewBakery(identchecker.BakeryParams{
			Locator:        discharger,
			Key:            bakery.MustGenerateKey(),
			IdentityClient: testIdentityClient{loc: discharger.Location()},
			Location:       "jimm",
			Logger:         testLogger{t: c},
		}),
		ControllerAdmins: []string{"bob"},
	}

	ctx := context.Background()
	u, err := authenticator.Authenticate(ctx, &jujuparams.LoginRequest{})
	c.Check(u, qt.IsNil)
	aerr, ok := err.(*auth.AuthenticationError)
	c.Assert(ok, qt.Equals, true, qt.Commentf("unexpected error %s", err))

	client := httpbakery.NewClient()
	ms, err := client.DischargeAll(ctx, aerr.LoginResult.BakeryDischargeRequired)
	c.Assert(err, qt.IsNil)
	u, err = authenticator.Authenticate(ctx, &jujuparams.LoginRequest{Macaroons: []macaroon.Slice{ms}})
	c.Assert(err, qt.IsNil)
	c.Check(u.LastLogin.Valid, qt.Equals, false)
	u.LastLogin = sql.NullTime{}
	c.Check(u, qt.DeepEquals, &dbmodel.User{
		Username:         "bob@external",
		DisplayName:      "bob",
		ControllerAccess: "superuser",
	})
}

func TestAuthenticateLoginInvalidUsernameDeclared(t *testing.T) {
	c := qt.New(t)

	discharger := bakerytest.NewDischarger(nil)
	c.Cleanup(discharger.Close)
	discharger.CheckerP = httpbakery.ThirdPartyCaveatCheckerPFunc(
		func(ctx context.Context, p httpbakery.ThirdPartyCaveatCheckerParams) ([]checkers.Caveat, error) {
			return []checkers.Caveat{checkers.DeclaredCaveat("username", "A")}, nil
		},
	)
	authenticator := auth.JujuAuthenticator{
		Bakery: identchecker.NewBakery(identchecker.BakeryParams{
			Locator:        discharger,
			Key:            bakery.MustGenerateKey(),
			IdentityClient: testIdentityClient{loc: discharger.Location()},
			Location:       "jimm",
			Logger:         testLogger{t: c},
		}),
	}

	ctx := context.Background()
	u, err := authenticator.Authenticate(ctx, &jujuparams.LoginRequest{})
	c.Check(u, qt.IsNil)
	aerr, ok := err.(*auth.AuthenticationError)
	c.Assert(ok, qt.Equals, true, qt.Commentf("unexpected error %s", err))

	client := httpbakery.NewClient()
	ms, err := client.DischargeAll(ctx, aerr.LoginResult.BakeryDischargeRequired)
	c.Assert(err, qt.IsNil)
	_, err = authenticator.Authenticate(ctx, &jujuparams.LoginRequest{Macaroons: []macaroon.Slice{ms}})
	c.Assert(err, qt.ErrorMatches, `authenticated identity "A" cannot be used as juju username`)
}

type testIdentityClient struct {
	loc string
}

func (c testIdentityClient) IdentityFromContext(ctx context.Context) (identchecker.Identity, []checkers.Caveat, error) {
	cav := checkers.Caveat{
		Condition: "is-authenticated-user",
		Location:  c.loc,
	}
	return nil, []checkers.Caveat{checkers.NeedDeclaredCaveat(cav, "username")}, nil
}

func (testIdentityClient) DeclaredIdentity(ctx context.Context, declared map[string]string) (identchecker.Identity, error) {
	if username, ok := declared["username"]; ok {
		return identchecker.SimpleIdentity(username), nil
	}
	return nil, errors.E("username not declared")
}

type testLogger struct {
	t testing.TB
}

func (l testLogger) Infof(_ context.Context, f string, args ...interface{}) {
	l.t.Logf(f, args...)
}

func (l testLogger) Debugf(_ context.Context, f string, args ...interface{}) {
	l.t.Logf(f, args...)
}
