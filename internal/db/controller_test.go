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
	"github.com/CanonicalLtd/jimm/internal/jimmtest"
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

func TestForEachControllerUnconfiguredDatabase(t *testing.T) {
	c := qt.New(t)

	var d db.Database
	err := d.ForEachController(context.Background(), nil)
	c.Check(err, qt.ErrorMatches, `database not configured`)
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeServerConfiguration)
}

const testForEachControllerEnv = `clouds:
- name: test
  type: test
  regions:
  - name: test-region
cloud-credentials:
- name: test-cred
  cloud: test
  owner: alice@external
  type: empty
controllers:
- name: test1
  uuid: 00000001-0000-0000-0000-000000000001
  cloud: test
  region: test-region-1
- name: test2
  uuid: 00000001-0000-0000-0000-000000000002
  cloud: test
  region: test-region-2
- name: test3
  uuid: 00000001-0000-0000-0000-000000000003
  cloud: test
  region: test-region-3
`

func (s *dbSuite) TestForEachController(c *qt.C) {
	ctx := context.Background()
	err := s.Database.Migrate(context.Background(), true)
	c.Assert(err, qt.Equals, nil)

	env := jimmtest.ParseEnvironment(c, testForEachControllerEnv)
	env.PopulateDB(c, *s.Database)

	testError := errors.E("test error")
	err = s.Database.ForEachController(ctx, func(controller *dbmodel.Controller) error {
		return testError
	})
	c.Check(err, qt.Equals, testError)

	var controllers []string
	err = s.Database.ForEachController(ctx, func(controller *dbmodel.Controller) error {
		controllers = append(controllers, controller.UUID)
		return nil
	})
	c.Assert(err, qt.IsNil)
	c.Check(controllers, qt.DeepEquals, []string{
		"00000001-0000-0000-0000-000000000001",
		"00000001-0000-0000-0000-000000000002",
		"00000001-0000-0000-0000-000000000003",
	})
}

func TestUpdateControllerUnconfiguredDatabase(t *testing.T) {
	c := qt.New(t)

	var d db.Database
	err := d.UpdateController(context.Background(), &dbmodel.Controller{})
	c.Check(err, qt.ErrorMatches, `database not configured`)
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeServerConfiguration)
}

func (s *dbSuite) TestUpdateController(c *qt.C) {
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

	controller.Deprecated = true
	controller.AgentVersion = "test-agent-version"
	controller.Addresses = []string{"test-addresses"}

	err = s.Database.UpdateController(context.Background(), &controller)
	c.Assert(err, qt.Equals, nil)

	dbController = dbmodel.Controller{
		Name: controller.Name,
	}
	err = s.Database.GetController(context.Background(), &dbController)
	c.Assert(err, qt.Equals, nil)
	c.Assert(dbController, qt.CmpEquals(cmpopts.EquateEmpty()), controller)

	err = s.Database.UpdateController(context.Background(), &dbmodel.Controller{})
	c.Assert(err, qt.Not(qt.IsNil))
	eError, ok := err.(*errors.Error)
	c.Assert(ok, qt.IsTrue)
	c.Assert(eError.Code, qt.Equals, errors.CodeNotFound)
}

func (s *dbSuite) TestDeleteController(c *qt.C) {
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

	err = s.Database.DeleteController(context.Background(), &controller)
	c.Assert(err, qt.Equals, nil)

	dbController = dbmodel.Controller{
		Name: controller.Name,
	}
	err = s.Database.GetController(context.Background(), &dbController)
	c.Assert(err, qt.ErrorMatches, "record not found")

	err = s.Database.DeleteController(context.Background(), &dbmodel.Controller{})
	c.Assert(err, qt.ErrorMatches, "controller not found")
}
