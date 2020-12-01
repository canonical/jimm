// Copyright 2020 Canonical Ltd.

package db_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	qt "github.com/frankban/quicktest"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/CanonicalLtd/jimm/internal/db"
	"github.com/CanonicalLtd/jimm/internal/dbmodel"
	"github.com/CanonicalLtd/jimm/internal/errors"
)

func TestAddControllerUnconfiguredDatabase(t *testing.T) {
	c := qt.New(t)

	var d db.Database
	err := d.AddController(context.Background(), &dbmodel.Controller{})
	c.Check(err, qt.ErrorMatches, `database not configured`)
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeServerConfiguration)
}

func (s *dbSuite) TestAddController(c *qt.C) {
	err := s.Database.Migrate(context.Background(), true)
	c.Assert(err, qt.Equals, nil)

	controller := dbmodel.Controller{
		Name: "test-controller",
		UUID: "00000000-0000-0000-0000-0000-0000000000001",
	}
	c1 := controller
	err = s.Database.AddController(context.Background(), &controller)
	c.Assert(err, qt.Equals, nil)

	var dbController dbmodel.Controller
	result := s.Database.DB.Where("uuid = ?", controller.UUID).First(&dbController)
	c.Assert(result.Error, qt.Equals, nil)
	c.Assert(dbController, qt.DeepEquals, controller)

	err = s.Database.AddController(context.Background(), &c1)
	c.Assert(err, qt.Not(qt.IsNil))
	eError, ok := err.(*errors.Error)
	c.Assert(ok, qt.IsTrue)
	c.Assert(eError.Code, qt.Equals, errors.CodeAlreadyExists)
}

func (s *dbSuite) TestGetController(c *qt.C) {
	err := s.Database.Migrate(context.Background(), true)
	c.Assert(err, qt.Equals, nil)

	controller := dbmodel.Controller{
		Name:   "test-controller",
		UUID:   "00000000-0000-0000-0000-0000-0000000000001",
		Models: []dbmodel.Model{},
	}
	err = s.Database.AddController(context.Background(), &controller)
	c.Assert(err, qt.Equals, nil)

	dbController := dbmodel.Controller{
		UUID: controller.UUID,
	}
	err = s.Database.GetController(context.Background(), &dbController)
	c.Assert(err, qt.Equals, nil)
	c.Assert(dbController, qt.CmpEquals(cmpopts.EquateEmpty()), controller)

	dbController = dbmodel.Controller{
		Name: controller.Name,
	}
	err = s.Database.GetController(context.Background(), &dbController)
	c.Assert(err, qt.Equals, nil)
	c.Assert(dbController, qt.CmpEquals(cmpopts.EquateEmpty()), controller)

	dbController = dbmodel.Controller{
		Name: "no such controller",
	}
	err = s.Database.GetController(context.Background(), &dbController)
	c.Assert(err, qt.Not(qt.IsNil))
	eError, ok := err.(*errors.Error)
	c.Assert(ok, qt.IsTrue)
	c.Assert(eError.Code, qt.Equals, errors.CodeNotFound)
}

func (s *dbSuite) TestGetControllerWithModels(c *qt.C) {
	err := s.Database.Migrate(context.Background(), true)
	c.Assert(err, qt.Equals, nil)

	controller := dbmodel.Controller{
		Name:   "test-controller",
		UUID:   "00000000-0000-0000-0000-0000-0000000000001",
		Models: []dbmodel.Model{},
	}
	u := dbmodel.User{
		Username: "bob@external",
	}
	c.Assert(s.Database.DB.Create(&u).Error, qt.IsNil)

	cloud := dbmodel.Cloud{
		Name: "test-cloud",
		Type: "test-provider",
		Regions: []dbmodel.CloudRegion{{
			Name: "test-region",
		}},
	}
	c.Assert(s.Database.DB.Create(&cloud).Error, qt.IsNil)

	cred := dbmodel.CloudCredential{
		Name:     "test-cred",
		Cloud:    cloud,
		Owner:    u,
		AuthType: "empty",
	}
	c.Assert(s.Database.DB.Create(&cred).Error, qt.IsNil)

	err = s.Database.AddController(context.Background(), &controller)
	c.Assert(err, qt.Equals, nil)

	models := []dbmodel.Model{{
		Name: "test-model-1",
		UUID: sql.NullString{
			String: "00000001-0000-0000-0000-0000-000000000001",
			Valid:  true,
		},
		Owner:           u,
		Controller:      controller,
		CloudRegion:     cloud.Regions[0],
		CloudCredential: cred,
		Type:            "iaas",
		IsController:    true,
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
		Users: []dbmodel.UserModelAccess{{
			User:   u,
			Access: "admin",
		}},
	}, {
		Name: "test-model-2",
		UUID: sql.NullString{
			String: "00000001-0000-0000-0000-0000-000000000002",
			Valid:  true,
		},
		Owner:           u,
		Controller:      controller,
		CloudRegion:     cloud.Regions[0],
		CloudCredential: cred,
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
		Users: []dbmodel.UserModelAccess{{
			User:   u,
			Access: "admin",
		}},
	}}
	for _, m := range models {
		c.Assert(s.Database.DB.Create(&m).Error, qt.IsNil)
	}

	dbController := dbmodel.Controller{
		UUID: controller.UUID,
	}
	err = s.Database.GetController(context.Background(), &dbController)
	c.Assert(err, qt.Equals, nil)
	controller.Models = []dbmodel.Model{
		models[0],
	}
	c.Assert(dbController, qt.CmpEquals(cmpopts.IgnoreFields(dbmodel.Controller{}, "Models"), cmpopts.EquateEmpty()), controller)
}
