// Copyright 2021 Canonical Ltd.

package jimm_test

import (
	"context"
	"sort"
	"testing"
	"time"

	"github.com/antonlindstrom/pgstore"
	qt "github.com/frankban/quicktest"
	"github.com/google/uuid"
	"github.com/juju/names/v5"

	"github.com/canonical/jimm/internal/auth"
	"github.com/canonical/jimm/internal/common/pagination"
	"github.com/canonical/jimm/internal/db"
	"github.com/canonical/jimm/internal/jimm"
	"github.com/canonical/jimm/internal/jimmtest"
	"github.com/canonical/jimm/internal/openfga"
	ofganames "github.com/canonical/jimm/internal/openfga/names"
)

func TestGetOpenFGAUser(t *testing.T) {
	c := qt.New(t)

	ctx := context.Background()

	// Test setup
	client, _, _, err := jimmtest.SetupTestOFGAClient(c.Name())
	c.Assert(err, qt.IsNil)

	db := &db.Database{
		DB: jimmtest.PostgresDB(c, func() time.Time { return time.Now() }),
	}
	sqldb, err := db.DB.DB()
	c.Assert(err, qt.IsNil)

	sessionStore, err := pgstore.NewPGStoreFromPool(sqldb, []byte("secretsecretdigletts"))
	c.Assert(err, qt.IsNil)
	authSvc, err := auth.NewAuthenticationService(ctx, auth.AuthenticationServiceParams{
		IssuerURL:           "http://localhost:8082/realms/jimm",
		ClientID:            "jimm-device",
		Scopes:              []string{"openid", "profile", "email"},
		SessionTokenExpiry:  time.Hour,
		Store:               db,
		SessionStore:        sessionStore,
		SessionCookieMaxAge: 60,
		JWTSessionKey:       "test-secret",
	})
	c.Assert(err, qt.IsNil)

	j := &jimm.JIMM{
		UUID:               "test",
		Database:           *db,
		OAuthAuthenticator: authSvc,
		OpenFGAClient:      client,
	}

	err = j.Database.Migrate(ctx, false)
	c.Assert(err, qt.IsNil)

	// Get the OpenFGA variant of the user
	ofgaUser, err := j.GetOpenFGAUserAndAuthorise(ctx, "bob@canonical.com.com")
	c.Assert(err, qt.IsNil)
	// Username -> email
	c.Assert(ofgaUser.Name, qt.Equals, "bob@canonical.com.com")
	// As no display name was set for this user as they're being created this time over
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

	ofgaUser, err = j.GetOpenFGAUserAndAuthorise(ctx, "bob@canonical.com.com")
	c.Assert(err, qt.IsNil)

	c.Assert(ofgaUser.Name, qt.Equals, "bob@canonical.com.com")
	c.Assert(ofgaUser.DisplayName, qt.Equals, "bob")
	c.Assert((time.Since(ofgaUser.LastLogin.Time) > time.Second), qt.IsFalse)
	c.Assert(ofgaUser.LastLogin.Valid, qt.IsTrue)
	// This user SHOULD be an admin, so ensure admin check is OK
	c.Assert(ofgaUser.JimmAdmin, qt.IsTrue)
}

func TestFetchIdentity(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()

	ofgaClient, _, _, err := jimmtest.SetupTestOFGAClient(c.Name())
	c.Assert(err, qt.IsNil)

	now := time.Now().UTC().Round(time.Millisecond)
	j := &jimm.JIMM{
		UUID: uuid.NewString(),
		Database: db.Database{
			DB: jimmtest.PostgresDB(c, func() time.Time { return now }),
		},
		OpenFGAClient: ofgaClient,
	}

	err = j.Database.Migrate(ctx, false)
	c.Assert(err, qt.IsNil)

	user, _, _, _, _, _, _ := createTestControllerEnvironment(ctx, c, j.Database)
	u, err := j.FetchUser(ctx, user.Name)
	c.Assert(err, qt.IsNil)
	c.Assert(u.Name, qt.Equals, user.Name)

	_, err = j.FetchUser(ctx, "bobnotfound@canonical.com")
	c.Assert(err, qt.ErrorMatches, "record not found")
}

func TestListUsers(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()

	ofgaClient, _, _, err := jimmtest.SetupTestOFGAClient(c.Name())
	c.Assert(err, qt.IsNil)

	now := time.Now().UTC().Round(time.Millisecond)
	j := &jimm.JIMM{
		UUID: uuid.NewString(),
		Database: db.Database{
			DB: jimmtest.PostgresDB(c, func() time.Time { return now }),
		},
		OpenFGAClient: ofgaClient,
	}

	err = j.Database.Migrate(ctx, false)
	c.Assert(err, qt.IsNil)

	user, _, _, _, _, _, _ := createTestControllerEnvironment(ctx, c, j.Database)

	u := openfga.NewUser(&user, ofgaClient)
	u.JimmAdmin = true

	filter := pagination.NewOffsetFilter(10, 0)
	users, err := j.ListUsers(ctx, u, filter)
	c.Assert(err, qt.IsNil)
	c.Assert(len(users), qt.Equals, 1)

	userNames := []string{
		"aabob1@canonical.com",
		"aabob3@canonical.com",
		"aabob5@canonical.com",
		"aabob4@canonical.com",
	}
	// add users
	for _, name := range userNames {
		_, err := j.GetUser(ctx, name)
		c.Assert(err, qt.IsNil)
	}
	users, err = j.ListUsers(ctx, u, filter)
	c.Assert(err, qt.IsNil)
	sort.Slice(users, func(i, j int) bool {
		return users[i].Name < users[j].Name
	})
	c.Assert(users, qt.HasLen, 5)
	// user should be returned in ascending order of name
	c.Assert(users[0].Name, qt.Equals, userNames[0])
	c.Assert(users[1].Name, qt.Equals, userNames[1])
	c.Assert(users[2].Name, qt.Equals, userNames[3])
	c.Assert(users[3].Name, qt.Equals, userNames[2])

	// test offset more than number of rows
	filter = pagination.NewOffsetFilter(10, 100)
	users, err = j.ListUsers(ctx, u, filter)
	c.Assert(err, qt.IsNil)
	c.Assert(users, qt.HasLen, 0)
}
