// Copyright 2024 Canonical Ltd.

package jimm_test

import (
	"context"
	"testing"

	qt "github.com/frankban/quicktest"
	"github.com/juju/names/v4"

	"github.com/canonical/jimm/internal/db"
	"github.com/canonical/jimm/internal/dbmodel"
	"github.com/canonical/jimm/internal/jimm"
	"github.com/canonical/jimm/internal/jimmtest"
	"github.com/canonical/jimm/internal/openfga"
	ofganames "github.com/canonical/jimm/internal/openfga/names"
	jimmnames "github.com/canonical/jimm/pkg/names"
	"github.com/canonical/ofga"
)

func TestAddServiceAccount(t *testing.T) {
	c := qt.New(t)

	ctx := context.Background()
	client, _, _, err := jimmtest.SetupTestOFGAClient(c.Name())
	c.Assert(err, qt.IsNil)
	j := &jimm.JIMM{
		OpenFGAClient: client,
	}
	c.Assert(err, qt.IsNil)
	user := openfga.NewUser(
		&dbmodel.Identity{
			Name:        "bob@external",
			DisplayName: "Bob",
		},
		client,
	)
	clientID := "39caae91-b914-41ae-83f8-c7b86ca5ad5a"
	err = j.AddServiceAccount(ctx, user, clientID)
	c.Assert(err, qt.IsNil)
	err = j.AddServiceAccount(ctx, user, clientID)
	c.Assert(err, qt.IsNil)
	userAlice := openfga.NewUser(
		&dbmodel.Identity{
			Name:        "alive@external",
			DisplayName: "Alice",
		},
		client,
	)
	err = j.AddServiceAccount(ctx, userAlice, clientID)
	c.Assert(err, qt.ErrorMatches, "service account already owned")
}

func TestGrantServiceAccountAccess(t *testing.T) {
	c := qt.New(t)

	tests := []struct {
		about                     string
		grantServiceAccountAccess func(ctx context.Context, user *openfga.User, tags []string) error
		clientID                  string
		tags                      []*ofganames.Tag
		username                  string
		expectedError             string
	}{{
		about: "Valid request",
		grantServiceAccountAccess: func(ctx context.Context, user *openfga.User, tags []string) error {
			return nil
		},
		tags: []*ofganames.Tag{
			&ofga.Entity{
				Kind: "user",
				ID:   "alice",
			},
			&ofga.Entity{
				Kind: "user",
				ID:   "bob",
			},
			&ofga.Entity{
				Kind:     "group",
				ID:       "1",
				Relation: "member",
			},
		},
		clientID: "fca1f605-736e-4d1f-bcd2-aecc726923be",
		username: "alice",
	}}

	for _, test := range tests {
		test := test
		c.Run(test.about, func(c *qt.C) {
			ofgaClient, _, _, err := jimmtest.SetupTestOFGAClient(c.Name())
			c.Assert(err, qt.IsNil)
			pgDb := db.Database{
				DB: jimmtest.PostgresDB(c, nil),
			}
			err = pgDb.Migrate(context.Background(), false)
			c.Assert(err, qt.IsNil)
			jimm := &jimm.JIMM{
				Database:      pgDb,
				OpenFGAClient: ofgaClient,
			}
			var u dbmodel.Identity
			u.SetTag(names.NewUserTag(test.clientID))
			svcAccountIdentity := openfga.NewUser(&u, ofgaClient)
			svcAccountTag := jimmnames.NewServiceAccountTag(test.clientID)

			err = jimm.GrantServiceAccountAccess(context.Background(), svcAccountIdentity, svcAccountTag, test.tags)
			if test.expectedError == "" {
				c.Assert(err, qt.IsNil)
				for _, tag := range test.tags {
					tuple := openfga.Tuple{
						Object:   tag,
						Relation: ofganames.AdministratorRelation,
						Target:   ofganames.ConvertTag(jimmnames.NewServiceAccountTag(test.clientID)),
					}
					ok, err := jimm.AuthorizationClient().CheckRelation(context.Background(), tuple, false)
					c.Assert(err, qt.IsNil)
					c.Assert(ok, qt.IsTrue)
				}
			} else {
				c.Assert(err, qt.ErrorMatches, test.expectedError)
			}
		})
	}
}
