// Copyright 2025 Canonical.

package offer_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	qt "github.com/frankban/quicktest"
	"github.com/google/uuid"
	"github.com/juju/names/v6"
	"go.uber.org/mock/gomock"

	"github.com/canonical/jimm/v3/internal/db"
	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/jimm/offer"
	offermocks "github.com/canonical/jimm/v3/internal/jimm/offer/mocks"
	"github.com/canonical/jimm/v3/internal/openfga"
	ofganames "github.com/canonical/jimm/v3/internal/openfga/names"
	"github.com/canonical/jimm/v3/internal/testutils/jimmtest"
	"github.com/canonical/jimm/v3/internal/testutils/testdb"
	jimmnames "github.com/canonical/jimm/v3/pkg/names"
)

type offerAuthorizerDeps struct {
	db              *db.Database
	ofgaClient      *openfga.OFGAClient
	offerAuthorizer *offer.OfferAuthorizer

	offerUUID string
}

const offerAuthEnv = `clouds:
- name: test-cloud
  type: test-provider
  regions:
  - name: test-region-1
cloud-credentials:
- owner: alice@canonical.com
  name: test-credential-1
  cloud: test-cloud
controllers:
- name: test-controller-1
  uuid: 00000000-0000-0000-0000-0000-0000000000001
  cloud: test-cloud
  region: test-region-1
models:
- name: test-model
  uuid: 00000000-0000-0000-0000-0000-0000000000003
  controller: test-controller-1
  cloud: test-cloud
  region: test-region-1
  cloud-credential: test-credential-1
  owner: alice@canonical.com
  life: alive
application-offers:
- name: test-offer
  url: test-offer-url
  uuid: 00000000-0000-0000-0000-0000-0000000000011
  model-name: test-model
  model-owner: alice@canonical.com
  application-name: application-1
  application-description: app description 1
  users:
  - user: eve@canonical.com
    access: admin
  - user: bob@canonical.com
    access: consume
`

func SetupOfferAuthorizerTests(c *qt.C) offerAuthorizerDeps {
	db := &db.Database{
		DB: testdb.PostgresDB(c, time.Now),
	}
	err := db.Migrate(context.Background())
	c.Assert(err, qt.IsNil)
	ofgaClient, _, _, err := jimmtest.SetupTestOFGAClient(c.Name())
	if err != nil {
		c.Fatalf("setting up openfga client: %v", err)
	}
	env := jimmtest.ParseEnvironment(c, offerAuthEnv)
	uuid := uuid.New()
	env.PopulateDBAndPermissions(c, names.NewControllerTag(uuid.String()), db, ofgaClient)
	ctrl := dbmodel.Controller{
		UUID: env.Controllers[0].UUID,
	}
	err = db.GetController(c.Context(), &ctrl)
	c.Assert(err, qt.IsNil)
	migration := dbmodel.UserMapping{
		ModelUUID:        sql.NullString{String: env.Models[0].UUID, Valid: true},
		LocalUser:        "bob",
		ExternalUserName: "bob@canonical.com",
	}
	err = db.AddUserMapping(c.Context(), &migration)
	c.Assert(err, qt.IsNil)

	deps := offerAuthorizerDeps{
		db:         db,
		ofgaClient: ofgaClient,
	}
	deps.offerAuthorizer, err = offer.NewOfferAuthorizer(db, ofgaClient, nil)
	c.Assert(err, qt.IsNil)

	deps.offerUUID = env.ApplicationOffers[0].UUID
	return deps
}

func TestIsUserConsumerForOffer(t *testing.T) {
	c := qt.New(t)
	deps := SetupOfferAuthorizerTests(c)

	tests := []struct {
		name                string
		userTag             names.UserTag
		applicationOfferTag names.ApplicationOfferTag
		allowed             bool
		expectedError       string
	}{
		{
			name:                "allowed external user",
			userTag:             names.NewUserTag("eve@canonical.com"),
			applicationOfferTag: names.NewApplicationOfferTag(deps.offerUUID),
			allowed:             true,
		},
		{
			name:                "not allowed external user",
			userTag:             names.NewUserTag("eve-not-allowed@canonical.com"),
			applicationOfferTag: names.NewApplicationOfferTag(deps.offerUUID),
			allowed:             false,
		},
		{
			name:                "not-existing application offer",
			userTag:             names.NewUserTag("eve@canonical.com"),
			applicationOfferTag: names.NewApplicationOfferTag("deeadbeef-dead-beef-dead-beefdeadbeef"),
			allowed:             false,
		},
		{
			name:                "allowed local user",
			userTag:             names.NewLocalUserTag("bob"),
			applicationOfferTag: names.NewApplicationOfferTag(deps.offerUUID),
			allowed:             true,
		},
		{
			name:                "not allowed local user",
			userTag:             names.NewLocalUserTag("alice"),
			applicationOfferTag: names.NewApplicationOfferTag(deps.offerUUID),
			allowed:             false,
			expectedError:       "user mapping not found",
		},
		{
			name:                "not-existing application offer local user",
			userTag:             names.NewUserTag("eve"),
			applicationOfferTag: names.NewApplicationOfferTag("deeadbeef-dead-beef-dead-beefdeadbeef"),
			allowed:             false,
			expectedError:       "^application offer not found.*",
		},
	}

	for _, test := range tests {
		c.Logf("Running test: %s", test.name)
		allowed, err := deps.offerAuthorizer.IsUserConsumerForOffer(c.Context(), test.userTag, test.applicationOfferTag)
		c.Assert(allowed, qt.Equals, test.allowed)
		if test.expectedError != "" {
			c.Assert(err, qt.ErrorMatches, test.expectedError)
		} else {
			c.Assert(err, qt.IsNil)
		}
	}
}

// TestIsUserConsumerForOfferViaIdPGroup verifies that a user whose consume
// access comes only from IdP group membership is authorized when the
// IdPGroupFetcher returns that group, and denied when it returns none.
func TestIsUserConsumerForOfferViaIdPGroup(t *testing.T) {
	c := qt.New(t)
	deps := SetupOfferAuthorizerTests(c)
	ctx := c.Context()

	user := "group-member@canonical.com"
	idpGroupName := "canonical"

	// Grant consume access on the offer to the IdP group.
	err := deps.ofgaClient.AddRelation(ctx, openfga.Tuple{
		Object:   ofganames.ConvertTagWithRelation(jimmnames.NewIdPGroupTag(idpGroupName), ofganames.MemberRelation),
		Relation: ofganames.ConsumerRelation,
		Target:   ofganames.ConvertTag(names.NewApplicationOfferTag(deps.offerUUID)),
	})
	c.Assert(err, qt.IsNil)

	tests := []struct {
		name    string
		groups  []string
		allowed bool
	}{
		{
			name:    "authorized because the group fetcher returns the group with consume access",
			groups:  []string{idpGroupName},
			allowed: true,
		},
		{
			name:    "not authorized because the group fetcher returns no groups",
			groups:  nil,
			allowed: false,
		},
	}

	for _, test := range tests {
		c.Run(test.name, func(c *qt.C) {
			ctrl := gomock.NewController(c)
			fetcher := offermocks.NewMockIdPGroupFetcher(ctrl)
			fetcher.EXPECT().FetchGroups(gomock.Any(), user).Return(test.groups, nil).Times(1)

			authorizer, err := offer.NewOfferAuthorizer(deps.db, deps.ofgaClient, fetcher)
			c.Assert(err, qt.IsNil)

			allowed, err := authorizer.IsUserConsumerForOffer(c.Context(), names.NewUserTag(user), names.NewApplicationOfferTag(deps.offerUUID))
			c.Assert(err, qt.IsNil)
			c.Assert(allowed, qt.Equals, test.allowed)
		})
	}
}
