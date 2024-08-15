// Copyright 2024 Canonical.

package dbmodel_test

import (
	"database/sql"
	"testing"

	qt "github.com/frankban/quicktest"
	"github.com/juju/juju/state"
	"github.com/juju/names/v5"

	"github.com/canonical/jimm/v3/internal/dbmodel"
)

func TestApplicationOfferTag(t *testing.T) {
	c := qt.New(t)

	ao := dbmodel.ApplicationOffer{
		UUID: "00000003-0000-0000-0000-0000-000000000001",
	}

	tag := ao.Tag()
	c.Check(tag.String(), qt.Equals, "applicationoffer-00000003-0000-0000-0000-0000-000000000001")

	var ao2 dbmodel.ApplicationOffer
	ao2.SetTag(tag.(names.ApplicationOfferTag))
	c.Check(ao2, qt.DeepEquals, ao)
}

func TestApplicationOfferUniqueConstraint(t *testing.T) {
	c := qt.New(t)
	db := gormDB(c)
	cl, cred, ctl, u := initModelEnv(c, db)

	m := dbmodel.Model{
		Name: "test-model",
		UUID: sql.NullString{
			String: "00000001-0000-0000-0000-0000-000000000001",
			Valid:  true,
		},
		Owner:           u,
		Controller:      ctl,
		CloudRegion:     cl.Regions[0],
		CloudCredential: cred,
		Type:            "iaas",
		IsController:    false,
		DefaultSeries:   "warty",
		Life:            state.Alive.String(),
	}
	c.Assert(db.Create(&m).Error, qt.IsNil)

	ao := dbmodel.ApplicationOffer{
		Name:  "offer1",
		UUID:  "00000003-0000-0000-0000-0000-000000000001",
		URL:   "foo",
		Model: m,
	}
	c.Assert(db.Create(&ao).Error, qt.IsNil)
	ao.ID = 0
	ao.Name = "offer2"
	ao.UUID = "00000003-0000-0000-0000-0000-000000000002"
	c.Assert(db.Create(&ao).Error, qt.ErrorMatches, `ERROR: duplicate key value violates unique constraint "application_offers_url_key" .*`)
}
