// Copyright 2025 Canonical.

package identity_test

import (
	"context"
	"testing"
	"time"

	qt "github.com/frankban/quicktest"
	"github.com/frankban/quicktest/qtsuite"

	"github.com/canonical/jimm/v3/internal/common/pagination"
	"github.com/canonical/jimm/v3/internal/db"
	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/jimm/identity"
	"github.com/canonical/jimm/v3/internal/openfga"
	"github.com/canonical/jimm/v3/internal/testutils/jimmtest"
)

type identityManagerSuite struct {
	manager    *identity.IdentityManager
	adminUser  *openfga.User
	db         *db.Database
	ofgaClient *openfga.OFGAClient
}

func (s *identityManagerSuite) Init(c *qt.C) {
	// Setup DB
	db := &db.Database{
		DB: jimmtest.PostgresDB(c, time.Now),
	}
	err := db.Migrate(context.Background())
	c.Assert(err, qt.IsNil)

	s.db = db

	// Setup OFGA
	ofgaClient, _, _, err := jimmtest.SetupTestOFGAClient(c.Name())
	c.Assert(err, qt.IsNil)

	s.ofgaClient = ofgaClient

	s.manager, err = identity.NewIdentityManager(db, ofgaClient)
	c.Assert(err, qt.IsNil)

	// Create test identity
	i, err := dbmodel.NewIdentity("alice")
	c.Assert(err, qt.IsNil)
	s.adminUser = openfga.NewUser(i, ofgaClient)
	s.adminUser.JimmAdmin = true
}

func (s *identityManagerSuite) TestFetchIdentity(c *qt.C) {
	c.Parallel()
	ctx := context.Background()

	identity := dbmodel.Identity{Name: "fake-name"}
	err := s.db.GetIdentity(ctx, &identity)
	c.Assert(err, qt.IsNil)
	u, err := s.manager.FetchIdentity(ctx, identity.Name)
	c.Assert(err, qt.IsNil)
	c.Assert(u.Name, qt.Equals, identity.Name)

	_, err = s.manager.FetchIdentity(ctx, "bobnotfound@canonical.com")
	c.Assert(err, qt.ErrorMatches, "record not found")
}

func (s *identityManagerSuite) TestListIdentities(c *qt.C) {
	c.Parallel()
	ctx := context.Background()

	pag := pagination.NewOffsetFilter(10, 0)
	users, err := s.manager.ListIdentities(ctx, s.adminUser, pag, "")
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
		identity := dbmodel.Identity{Name: name}
		err := s.db.GetIdentity(ctx, &identity)
		c.Assert(err, qt.IsNil)
	}

	testCases := []struct {
		desc       string
		limit      int
		offset     int
		match      string
		identities []string
	}{
		{
			desc:       "test with first ids",
			limit:      3,
			offset:     0,
			identities: []string{userNames[0], userNames[1], userNames[3]},
		},
		{
			desc:       "test with remianing ids",
			limit:      3,
			offset:     3,
			identities: []string{userNames[2]},
		},
		{
			desc:       "test out of range",
			limit:      3,
			offset:     6,
			identities: []string{},
		},
		{
			desc:       "test with match",
			limit:      5,
			offset:     0,
			identities: []string{userNames[0]},
			match:      "bob1",
		},
	}
	for _, t := range testCases {
		c.Run(t.desc, func(c *qt.C) {
			pag = pagination.NewOffsetFilter(t.limit, t.offset)
			identities, err := s.manager.ListIdentities(ctx, s.adminUser, pag, t.match)
			c.Assert(err, qt.IsNil)
			c.Assert(identities, qt.HasLen, len(t.identities))
			for i := range len(t.identities) {
				c.Assert(identities[i].Name, qt.Equals, t.identities[i])
			}
		})
	}
}

func (s *identityManagerSuite) TestCountIdentities(c *qt.C) {
	c.Parallel()
	ctx := context.Background()

	userNames := []string{
		"bob1@canonical.com",
		"bob3@canonical.com",
		"bob5@canonical.com",
		"bob4@canonical.com",
	}
	// add users
	for _, name := range userNames {
		identity := dbmodel.Identity{Name: name}
		err := s.db.GetIdentity(ctx, &identity)
		c.Assert(err, qt.IsNil)
	}
	count, err := s.manager.CountIdentities(ctx, s.adminUser)
	c.Assert(err, qt.IsNil)
	c.Assert(count, qt.Equals, 4)
}

func TestIdentityManager(t *testing.T) {
	qtsuite.Run(qt.New(t), &identityManagerSuite{})
}
