// Copyright 2025 Canonical.

package offer_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	qt "github.com/frankban/quicktest"
	"github.com/frankban/quicktest/qtsuite"
	"github.com/google/uuid"
	"github.com/juju/names/v5"
	gc "gopkg.in/check.v1"

	"github.com/canonical/jimm/v3/internal/db"
	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/jimm/offer"
	"github.com/canonical/jimm/v3/internal/testutils/jimmtest"
)

type offerAuthorizerSuite struct {
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

var _ = gc.Suite(&offerAuthorizerSuite{})

func (s *offerAuthorizerSuite) Init(c *qt.C) {
	db := &db.Database{
		DB: jimmtest.PostgresDB(c, time.Now),
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

	s.offerAuthorizer, err = offer.NewOfferAuthorizer(db, ofgaClient)
	c.Assert(err, qt.IsNil)

	s.offerUUID = env.ApplicationOffers[0].UUID
}

func (s *offerAuthorizerSuite) TestIsUserConsumerForOffer(c *qt.C) {
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
			applicationOfferTag: names.NewApplicationOfferTag(s.offerUUID),
			allowed:             true,
		},
		{
			name:                "not allowed external user",
			userTag:             names.NewUserTag("eve-not-allowed@canonical.com"),
			applicationOfferTag: names.NewApplicationOfferTag(s.offerUUID),
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
			applicationOfferTag: names.NewApplicationOfferTag(s.offerUUID),
			allowed:             true,
		},
		{
			name:                "not allowed local user",
			userTag:             names.NewLocalUserTag("alice"),
			applicationOfferTag: names.NewApplicationOfferTag(s.offerUUID),
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
		allowed, err := s.offerAuthorizer.IsUserConsumerForOffer(c.Context(), test.userTag, test.applicationOfferTag)
		c.Assert(allowed, qt.Equals, test.allowed)
		if test.expectedError != "" {
			c.Assert(err, qt.ErrorMatches, test.expectedError)
		} else {
			c.Assert(err, qt.IsNil)
		}
	}
}

func TestOfferAuthorizerSuite(t *testing.T) {
	qtsuite.Run(qt.New(t), &offerAuthorizerSuite{})
}
