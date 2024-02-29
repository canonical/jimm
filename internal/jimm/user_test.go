// Copyright 2021 Canonical Ltd.

package jimm_test

import (
	"context"
	"testing"
	"time"

	qt "github.com/frankban/quicktest"
	"github.com/juju/names/v5"

	"github.com/canonical/jimm/internal/auth"
	"github.com/canonical/jimm/internal/db"
	"github.com/canonical/jimm/internal/jimm"
	"github.com/canonical/jimm/internal/jimmtest"
	ofganames "github.com/canonical/jimm/internal/openfga/names"
)

func TestGetOpenFGAUser(t *testing.T) {
	c := qt.New(t)

	ctx := context.Background()

	// Test setup
	client, _, _, err := jimmtest.SetupTestOFGAClient(c.Name())
	c.Assert(err, qt.IsNil)

	// TODO(ale8k): Mock this
	authSvc, err := auth.NewAuthenticationService(ctx, auth.AuthenticationServiceParams{
		IssuerURL:          "http://localhost:8082/realms/jimm",
		ClientID:           "jimm-device",
		Scopes:             []string{"openid", "profile", "email"},
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
	c.Assert(ofgaUser.Name, qt.Equals, "bob@external.com")
	// As no display name was set for this user as they're being created this time over
	c.Assert(ofgaUser.DisplayName, qt.Equals, "")
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

	c.Assert(ofgaUser.Name, qt.Equals, "bob@external.com")
	c.Assert(ofgaUser.DisplayName, qt.Equals, "")
	c.Assert((time.Since(ofgaUser.LastLogin.Time) > time.Second), qt.IsFalse)
	c.Assert(ofgaUser.LastLogin.Valid, qt.IsTrue)
	// This user SHOULD be an admin, so ensure admin check is OK
	c.Assert(ofgaUser.JimmAdmin, qt.IsTrue)
}
