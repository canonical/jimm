// Copyright 2021 Canonical Ltd.

package jimm_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	qt "github.com/frankban/quicktest"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v4"

	"github.com/canonical/jimm/internal/auth"
	"github.com/canonical/jimm/internal/db"
	"github.com/canonical/jimm/internal/dbmodel"
	"github.com/canonical/jimm/internal/errors"
	"github.com/canonical/jimm/internal/jimm"
	"github.com/canonical/jimm/internal/jimmtest"
	"github.com/canonical/jimm/internal/openfga"
	ofganames "github.com/canonical/jimm/internal/openfga/names"
)

func TestAuthenticateNoAuthenticator(t *testing.T) {
	c := qt.New(t)

	j := &jimm.JIMM{}
	_, err := j.Authenticate(context.Background(), &jujuparams.LoginRequest{})
	c.Check(err, qt.ErrorMatches, `authenticator not configured`)
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeServerConfiguration)
}

func TestAuthenticate(t *testing.T) {
	c := qt.New(t)

	client, _, _, err := jimmtest.SetupTestOFGAClient(c.Name())
	c.Assert(err, qt.IsNil)

	var auth jimmtest.Authenticator
	now := time.Now().UTC().Round(time.Millisecond)
	j := &jimm.JIMM{
		UUID: "test",
		Database: db.Database{
			DB: jimmtest.PostgresDB(c, func() time.Time { return now }),
		},
		Authenticator: &auth,
		OpenFGAClient: client,
	}
	ctx := context.Background()

	err = j.Database.Migrate(ctx, false)
	c.Assert(err, qt.IsNil)

	auth.User = openfga.NewUser(
		&dbmodel.User{
			Username:    "bob@external",
			DisplayName: "Bob",
		},
		client,
	)
	u, err := j.Authenticate(ctx, nil)
	c.Assert(err, qt.IsNil)
	c.Check(u.Username, qt.Equals, "bob@external")
	c.Check(u.JimmAdmin, qt.IsFalse)

	err = auth.User.SetControllerAccess(
		context.Background(),
		names.NewControllerTag(j.UUID),
		ofganames.AdministratorRelation,
	)
	c.Assert(err, qt.IsNil)

	u, err = j.Authenticate(ctx, nil)
	c.Assert(err, qt.IsNil)
	c.Check(u.Username, qt.Equals, "bob@external")
	c.Check(u.JimmAdmin, qt.IsTrue)

	u2 := dbmodel.User{
		Username: "bob@external",
	}
	err = j.Database.GetUser(ctx, &u2)
	c.Assert(err, qt.IsNil)

	c.Check(u2, qt.DeepEquals, dbmodel.User{
		Model:       u.Model,
		Username:    "bob@external",
		DisplayName: "Bob",
		LastLogin: sql.NullTime{
			Time:  now,
			Valid: true,
		},
	})

	auth.Err = errors.E("test error", errors.CodeUnauthorized)
	u, err = j.Authenticate(ctx, nil)
	c.Check(err, qt.ErrorMatches, `test error`)
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeUnauthorized)
	c.Check(u, qt.IsNil)
}

func TestGetOpenFGAUser(t *testing.T) {
	c := qt.New(t)

	ctx := context.Background()

	// Test setup
	client, _, _, err := jimmtest.SetupTestOFGAClient(c.Name())
	c.Assert(err, qt.IsNil)

	// TODO(ale8k): Mock this
	authSvc, err := auth.NewAuthenticationService(ctx, auth.AuthenticationServiceParams{
		IssuerURL:          "http://localhost:8082/realms/jimm",
		DeviceClientID:     "jimm-device",
		DeviceScopes:       []string{"openid", "profile", "email"},
		SessionTokenExpiry: time.Hour,
	})
	c.Assert(err, qt.IsNil)

	now := time.Now().UTC().Round(time.Millisecond)
	j := &jimm.JIMM{
		UUID: "test",
		Database: db.Database{
			DB: jimmtest.PostgresDB(c, func() time.Time { return now }),
		},
		OAuthAuthenticator: authSvc,
		OpenFGAClient:      client,
	}

	err = j.Database.Migrate(ctx, false)
	c.Assert(err, qt.IsNil)

	// Get the OpenFGA variant of the user
	ofgaUser, err := j.GetOpenFGAUserAndAuthorise(ctx, "bob@external.com")
	c.Assert(err, qt.IsNil)
	// Username -> email
	c.Assert(ofgaUser.Username, qt.Equals, "bob@external.com")
	// As no display name was set for this user, display name -> (username - domain)
	c.Assert(ofgaUser.DisplayName, qt.Equals, "bob")
	// The last login should be updated, so we check if it's been updated
	// in the last second (for general accuracy when testing)
	c.Assert((time.Since(ofgaUser.LastLogin.Time) > time.Second), qt.IsFalse)
	// Ensure last login was valid
	c.Assert(ofgaUser.LastLogin.Valid, qt.IsTrue)
	// This user SHOULD NOT be an admin, so ensure admin check is OK
	c.Assert(ofgaUser.JimmAdmin, qt.IsFalse)

	// Next we'll update this user to an admin of JIMM and run the same tests.
	c.Assert(
		ofgaUser.SetControllerAccess(
			context.Background(),
			names.NewControllerTag(j.UUID),
			ofganames.AdministratorRelation,
		),
		qt.IsNil,
	)

	ofgaUser, err = j.GetOpenFGAUserAndAuthorise(ctx, "bob@external.com")
	c.Assert(err, qt.IsNil)

	c.Assert(ofgaUser.Username, qt.Equals, "bob@external.com")
	c.Assert(ofgaUser.DisplayName, qt.Equals, "bob")
	c.Assert((time.Since(ofgaUser.LastLogin.Time) > time.Second), qt.IsFalse)
	c.Assert(ofgaUser.LastLogin.Valid, qt.IsTrue)
	// This user SHOULD be an admin, so ensure admin check is OK
	c.Assert(ofgaUser.JimmAdmin, qt.IsTrue)

}
