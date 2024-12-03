// Copyright 2024 Canonical.

package db_test

import (
	"context"
	"database/sql"
	"sort"
	"testing"

	qt "github.com/frankban/quicktest"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/juju/juju/state"

	"github.com/canonical/jimm/v3/internal/db"
	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
)

func TestAddApplicationOfferUnconfiguredDatabase(t *testing.T) {
	c := qt.New(t)
	var d db.Database

	err := d.AddApplicationOffer(context.Background(), &dbmodel.ApplicationOffer{})
	c.Check(err, qt.ErrorMatches, `database not configured`)
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeServerConfiguration)
}

type testEnvironment struct {
	u          dbmodel.Identity
	cloud      dbmodel.Cloud
	cred       dbmodel.CloudCredential
	controller dbmodel.Controller
	model      dbmodel.Model
	model1     dbmodel.Model
}

func initTestEnvironment(c *qt.C, db *db.Database) testEnvironment {
	err := db.Migrate(context.Background(), true)
	c.Assert(err, qt.Equals, nil)

	env := testEnvironment{}
	i, err := dbmodel.NewIdentity("bob@canonical.com")
	c.Assert(err, qt.IsNil)
	env.u = *i

	c.Assert(db.DB.Create(&env.u).Error, qt.IsNil)

	env.cloud = dbmodel.Cloud{
		Name: "test-cloud",
		Type: "test-provider",
		Regions: []dbmodel.CloudRegion{{
			Name: "test-region",
		}},
	}
	c.Assert(db.DB.Create(&env.cloud).Error, qt.IsNil)

	env.cred = dbmodel.CloudCredential{
		Name:     "test-cred",
		Cloud:    env.cloud,
		Owner:    env.u,
		AuthType: "empty",
	}
	c.Assert(db.DB.Create(&env.cred).Error, qt.IsNil)

	env.controller = dbmodel.Controller{
		Name:        "test-controller",
		UUID:        "00000000-0000-0000-0000-0000-0000000000001",
		CloudName:   "test-cloud",
		CloudRegion: "test-region",
	}
	c.Assert(db.DB.Create(&env.controller).Error, qt.IsNil)

	env.model = dbmodel.Model{
		Name: "test-model",
		UUID: sql.NullString{
			String: "00000001-0000-0000-0000-0000-000000000001",
			Valid:  true,
		},
		Owner:           env.u,
		Controller:      env.controller,
		CloudRegion:     env.cloud.Regions[0],
		CloudCredential: env.cred,
		Life:            state.Alive.String(),
	}
	c.Assert(db.DB.Create(&env.model).Error, qt.IsNil)

	env.model1 = dbmodel.Model{
		Name: "test-model-2",
		UUID: sql.NullString{
			String: "00000001-0000-0000-0000-0000-000000000002",
			Valid:  true,
		},
		Owner:           env.u,
		Controller:      env.controller,
		CloudRegion:     env.cloud.Regions[0],
		CloudCredential: env.cred,
		Life:            state.Alive.String(),
	}
	c.Assert(db.DB.Create(&env.model1).Error, qt.IsNil)

	return env
}

func (s *dbSuite) TestAddApplicationOffer(c *qt.C) {
	env := initTestEnvironment(c, s.Database)

	offer := dbmodel.ApplicationOffer{
		Name:    "offer1",
		UUID:    "00000000-0000-0000-0000-000000000001",
		ModelID: env.model.ID,
	}
	err := s.Database.AddApplicationOffer(context.Background(), &offer)
	c.Assert(err, qt.Equals, nil)

	var dbOffer dbmodel.ApplicationOffer
	result := s.Database.DB.Where("uuid = ?", offer.UUID).First(&dbOffer)
	c.Assert(result.Error, qt.Equals, nil)
	c.Assert(dbOffer, qt.DeepEquals, offer)
}

func TestGetApplicationOfferUnconfiguredDatabase(t *testing.T) {
	c := qt.New(t)

	var d db.Database
	err := d.GetApplicationOffer(context.Background(), &dbmodel.ApplicationOffer{})
	c.Check(err, qt.ErrorMatches, `database not configured`)
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeServerConfiguration)
}

func (s *dbSuite) TestGetApplicationOffer(c *qt.C) {
	env := initTestEnvironment(c, s.Database)

	offer := dbmodel.ApplicationOffer{
		Name:    "offer",
		UUID:    "00000000-0000-0000-0000-000000000001",
		ModelID: env.model.ID,
	}
	err := s.Database.AddApplicationOffer(context.Background(), &offer)
	c.Assert(err, qt.Equals, nil)

	dbOffer := dbmodel.ApplicationOffer{
		UUID: "00000000-0000-0000-0000-000000000001",
	}

	err = s.Database.GetApplicationOffer(context.Background(), &dbOffer)
	c.Assert(err, qt.Equals, nil)
	c.Assert(dbOffer, qt.CmpEquals(cmpopts.EquateEmpty(), cmpopts.IgnoreTypes(dbmodel.Model{})), offer)

	dbOffer = dbmodel.ApplicationOffer{
		UUID: "00000000-0000-0000-0000-000000000002",
	}
	err = s.Database.GetApplicationOffer(context.Background(), &dbOffer)
	c.Assert(errors.ErrorCode(err), qt.Equals, errors.CodeNotFound)
}

func (s *dbSuite) TestDeleteApplicationOffer(c *qt.C) {
	env := initTestEnvironment(c, s.Database)

	offer := dbmodel.ApplicationOffer{
		UUID:    "00000000-0000-0000-0000-000000000001",
		ModelID: env.model.ID,
	}
	err := s.Database.AddApplicationOffer(context.Background(), &offer)
	c.Assert(err, qt.Equals, nil)

	err = s.Database.DeleteApplicationOffer(context.Background(), &offer)
	c.Assert(err, qt.Equals, nil)

	dbOffer := dbmodel.ApplicationOffer{
		UUID: "00000000-0000-0000-0000-000000000001",
	}
	err = s.Database.GetApplicationOffer(context.Background(), &dbOffer)
	c.Assert(errors.ErrorCode(err), qt.Equals, errors.CodeNotFound)
}

func (s *dbSuite) TestFindApplicationOffersByModel(c *qt.C) {
	env := initTestEnvironment(c, s.Database)

	offer1 := dbmodel.ApplicationOffer{
		UUID:    "00000000-0000-0000-0000-000000000001",
		Name:    "offer-1",
		ModelID: env.model.ID,
		URL:     "url-1",
	}
	err := s.Database.AddApplicationOffer(context.Background(), &offer1)
	c.Assert(err, qt.Equals, nil)

	offer2 := dbmodel.ApplicationOffer{
		UUID:    "00000000-0000-0000-0000-000000000002",
		Name:    "offer-2",
		ModelID: env.model1.ID,
		URL:     "url-2",
	}
	err = s.Database.AddApplicationOffer(context.Background(), &offer2)
	c.Assert(err, qt.Equals, nil)

	offer3 := dbmodel.ApplicationOffer{
		UUID:    "00000000-0000-0000-0000-000000000003",
		Name:    "test-3",
		ModelID: env.model1.ID,
		URL:     "url-3",
	}
	err = s.Database.AddApplicationOffer(context.Background(), &offer3)
	c.Assert(err, qt.Equals, nil)

	offers, err := s.Database.FindApplicationOffersByModel(context.Background(), env.model.Name, env.u.Name)
	c.Assert(err, qt.IsNil)
	c.Assert(offers, qt.CmpEquals(cmpopts.IgnoreTypes(dbmodel.Model{})), []dbmodel.ApplicationOffer{offer1})
	c.Assert(offers[0].Model.UUID, qt.Equals, env.model.UUID)

	offers, err = s.Database.FindApplicationOffersByModel(context.Background(), env.model1.Name, env.u.Name)
	c.Assert(err, qt.IsNil)
	sort.Slice(offers, func(i, j int) bool {
		return offers[i].Name < offers[j].Name
	})
	c.Assert(offers, qt.CmpEquals(cmpopts.IgnoreTypes(dbmodel.Model{})), []dbmodel.ApplicationOffer{offer2, offer3})

	offers, err = s.Database.FindApplicationOffersByModel(context.Background(), "no-such-model", env.u.Name)
	c.Assert(err, qt.IsNil)
	c.Assert(offers, qt.HasLen, 0)
}
