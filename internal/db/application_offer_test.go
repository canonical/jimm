// Copyright 2020 Canonical Ltd.

package db_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	qt "github.com/frankban/quicktest"

	"github.com/CanonicalLtd/jimm/internal/db"
	"github.com/CanonicalLtd/jimm/internal/dbmodel"
	"github.com/CanonicalLtd/jimm/internal/errors"
)

func TestAddApplicationOfferUnconfiguredDatabase(t *testing.T) {
	c := qt.New(t)

	var d db.Database
	err := d.AddApplicationOffer(context.Background(), &dbmodel.ApplicationOffer{})
	c.Check(err, qt.ErrorMatches, `database not configured`)
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeServerConfiguration)
}

type testEnvironment struct {
	u          dbmodel.User
	cloud      dbmodel.Cloud
	cred       dbmodel.CloudCredential
	controller dbmodel.Controller
	model      dbmodel.Model
}

func initTestEnvironment(c *qt.C, db *db.Database) testEnvironment {
	err := db.Migrate(context.Background(), true)
	c.Assert(err, qt.Equals, nil)

	env := testEnvironment{}

	env.u = dbmodel.User{
		Username: "bob@external",
	}
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
		Name: "test-controller",
		UUID: "00000000-0000-0000-0000-0000-0000000000001",
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
		Type:            "iaas",
		IsController:    false,
		DefaultSeries:   "warty",
		Life:            "alive",
		Status: dbmodel.Status{
			Status: "available",
			Since: sql.NullTime{
				Time:  time.Now(),
				Valid: true,
			},
		},
		SLA: dbmodel.SLA{
			Level: "unsupported",
		},
		Applications: []dbmodel.Application{{
			Name:     "app-1",
			Exposed:  true,
			CharmURL: "ch:charm",
			Life:     "alive",
			MinUnits: 1,
			Constraints: dbmodel.Constraints{
				Arch: sql.NullString{
					String: "amd64",
					Valid:  true,
				},
			},
			Config: dbmodel.Map(map[string]interface{}{
				"a": "b",
			}),
			Subordinate: false,
			Status: dbmodel.Status{
				Status: "active",
				Info:   "Unit Is Ready",
				Since: sql.NullTime{
					Time:  time.Now(),
					Valid: true,
				},
			},
			WorkloadVersion: "1.0.0",
		}},
		Machines: []dbmodel.Machine{{
			MachineID: "0",
			Hardware: dbmodel.MachineHardware{
				Arch: sql.NullString{
					String: "amd64",
					Valid:  true,
				},
				Mem: dbmodel.NullUint64{
					Uint64: 2000,
					Valid:  true,
				},
			},
			InstanceID:  "test-machine-0",
			DisplayName: "Machine 0",
			AgentStatus: dbmodel.Status{
				Status: "started",
				Since: sql.NullTime{
					Time:  time.Now(),
					Valid: true,
				},
				Version: "1.0.0",
			},
			InstanceStatus: dbmodel.Status{
				Status: "running",
				Info:   "ACTIVE",
				Since: sql.NullTime{
					Time:  time.Now(),
					Valid: true,
				},
			},
			Series: "warty",
		}},
		Users: []dbmodel.UserModelAccess{{
			User:   env.u,
			Access: "admin",
		}},
	}
	c.Assert(db.DB.Create(&env.model).Error, qt.IsNil)

	return env
}

func (s *dbSuite) TestAddApplicationOffer(c *qt.C) {
	env := initTestEnvironment(c, s.Database)

	offer := dbmodel.ApplicationOffer{
		UUID:          "00000000-0000-0000-0000-000000000001",
		ApplicationID: env.model.Applications[0].ID,
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
		UUID:          "00000000-0000-0000-0000-000000000001",
		ApplicationID: env.model.Applications[0].ID,
		Users: []dbmodel.UserApplicationOfferAccess{{
			UserID: env.u.ID,
			Access: "admin",
		}},
	}
	err := s.Database.AddApplicationOffer(context.Background(), &offer)
	c.Assert(err, qt.Equals, nil)

	dbOffer := dbmodel.ApplicationOffer{
		UUID: "00000000-0000-0000-0000-000000000001",
	}

	err = s.Database.GetApplicationOffer(context.Background(), &dbOffer)
	c.Assert(err, qt.Equals, nil)
	c.Assert(dbOffer, qt.DeepEquals, offer)

	dbOffer = dbmodel.ApplicationOffer{
		UUID: "00000000-0000-0000-0000-000000000002",
	}
	err = s.Database.GetApplicationOffer(context.Background(), &dbOffer)
	c.Assert(errors.ErrorCode(err), qt.Equals, errors.CodeNotFound)
}

func (s *dbSuite) TestUpdateApplicationOffer(c *qt.C) {
	env := initTestEnvironment(c, s.Database)

	offer := dbmodel.ApplicationOffer{
		UUID:          "00000000-0000-0000-0000-000000000001",
		ApplicationID: env.model.Applications[0].ID,
		Users:         []dbmodel.UserApplicationOfferAccess{},
	}
	err := s.Database.AddApplicationOffer(context.Background(), &offer)
	c.Assert(err, qt.Equals, nil)

	dbOffer := dbmodel.ApplicationOffer{
		UUID: "00000000-0000-0000-0000-000000000001",
	}
	err = s.Database.GetApplicationOffer(context.Background(), &dbOffer)
	c.Assert(err, qt.Equals, nil)
	c.Logf("%#v %#v", dbOffer.Users, offer.Users)
	c.Assert(dbOffer, qt.DeepEquals, offer)

	offer1 := offer
	offer1.Users = []dbmodel.UserApplicationOfferAccess{{
		UserID: env.u.ID,
		Access: "read",
	}}
	err = s.Database.UpdateApplicationOffer(context.Background(), &offer1)
	c.Assert(err, qt.Equals, nil)

	dbOffer = dbmodel.ApplicationOffer{
		UUID: "00000000-0000-0000-0000-000000000001",
	}
	err = s.Database.GetApplicationOffer(context.Background(), &dbOffer)
	c.Assert(err, qt.Equals, nil)
	c.Assert(dbOffer, qt.DeepEquals, offer1)

	offer2 := offer
	offer2.Users = []dbmodel.UserApplicationOfferAccess{{
		UserID: env.u.ID,
		Access: "admin",
	}}
	err = s.Database.UpdateApplicationOffer(context.Background(), &offer2)
	c.Assert(err, qt.Equals, nil)

	err = s.Database.GetApplicationOffer(context.Background(), &dbOffer)
	c.Assert(err, qt.Equals, nil)
	c.Assert(dbOffer, qt.DeepEquals, offer2)

	offer3 := dbmodel.ApplicationOffer{
		UUID:          "00000000-0000-0000-0000-000000000002",
		ApplicationID: env.model.Applications[0].ID,
	}
	err = s.Database.UpdateApplicationOffer(context.Background(), &offer3)
	c.Assert(err, qt.Not(qt.IsNil))
}

func (s *dbSuite) TestDeleteApplicationOffer(c *qt.C) {
	env := initTestEnvironment(c, s.Database)

	offer := dbmodel.ApplicationOffer{
		UUID:          "00000000-0000-0000-0000-000000000001",
		ApplicationID: env.model.Applications[0].ID,
	}
	err := s.Database.AddApplicationOffer(context.Background(), &offer)
	c.Assert(err, qt.Equals, nil)

	err = s.Database.DeleteApplicationOffer(context.Background(), &offer)
	c.Assert(err, qt.Equals, nil)

	dbOffer := dbmodel.ApplicationOffer{
		UUID:  "00000000-0000-0000-0000-000000000001",
		Users: nil,
	}
	err = s.Database.GetApplicationOffer(context.Background(), &dbOffer)
	c.Assert(errors.ErrorCode(err), qt.Equals, errors.CodeNotFound)
}
