// Copyright 2021 Canonical Ltd.

package jimm_test

import (
	"context"
	"sort"
	"testing"
	"time"

	qt "github.com/frankban/quicktest"
	"github.com/google/uuid"

	"github.com/canonical/jimm/v3/internal/common/pagination"
	"github.com/canonical/jimm/v3/internal/db"
	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/jimm"
	"github.com/canonical/jimm/v3/internal/jimmtest"
	"github.com/canonical/jimm/v3/internal/openfga"
)

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
	u, err := j.FetchIdentity(ctx, user.Name)
	c.Assert(err, qt.IsNil)
	c.Assert(u.Name, qt.Equals, user.Name)

	_, err = j.FetchIdentity(ctx, "bobnotfound@canonical.com")
	c.Assert(err, qt.ErrorMatches, "record not found")
}

func TestListIdentities(t *testing.T) {
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

	u := openfga.NewUser(&dbmodel.Identity{Name: "admin@canonical.com"}, ofgaClient)
	u.JimmAdmin = true

	filter := pagination.NewOffsetFilter(10, 0)
	users, err := j.ListIdentities(ctx, u, filter)
	c.Assert(err, qt.IsNil)
	c.Assert(len(users), qt.Equals, 0)

	userNames := []string{
		"bob1@canonical.com",
		"bob3@canonical.com",
		"bob5@canonical.com",
		"bob4@canonical.com",
	}
	// add users
	for _, name := range userNames {
		_, err := j.GetUser(ctx, name)
		c.Assert(err, qt.IsNil)
	}

	// test users returned
	filter = pagination.NewOffsetFilter(3, 0)
	users, err = j.ListIdentities(ctx, u, filter)
	c.Assert(err, qt.IsNil)
	sort.Slice(users, func(i, j int) bool {
		return users[i].Name < users[j].Name
	})
	c.Assert(users, qt.HasLen, 3)
	// user should be returned in ascending order of name
	c.Assert(users[0].Name, qt.Equals, userNames[0])
	c.Assert(users[1].Name, qt.Equals, userNames[1])
	c.Assert(users[2].Name, qt.Equals, userNames[3])

	// test remaining users
	filter = pagination.NewOffsetFilter(3, 3)
	users, err = j.ListIdentities(ctx, u, filter)
	c.Assert(err, qt.IsNil)
	c.Assert(users, qt.HasLen, 1)
	// user should be returned in ascending order of name
	c.Assert(users[0].Name, qt.Equals, userNames[2])

	// test offset more than number of rows
	filter = pagination.NewOffsetFilter(3, 5)
	users, err = j.ListIdentities(ctx, u, filter)
	c.Assert(err, qt.IsNil)
	c.Assert(users, qt.HasLen, 0)
}

func TestCountIdentities(t *testing.T) {
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

	u := openfga.NewUser(&dbmodel.Identity{Name: "admin@canonical.com"}, ofgaClient)
	u.JimmAdmin = true

	userNames := []string{
		"bob1@canonical.com",
		"bob3@canonical.com",
		"bob5@canonical.com",
		"bob4@canonical.com",
	}
	// add users
	for _, name := range userNames {
		_, err := j.GetUser(ctx, name)
		c.Assert(err, qt.IsNil)
	}
	count, err := j.CountIdentities(ctx, u)
	c.Assert(err, qt.IsNil)
	c.Assert(count, qt.Equals, 4)
}
