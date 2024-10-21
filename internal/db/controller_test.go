// Copyright 2024 Canonical.

package db_test

import (
	"context"
	"testing"

	qt "github.com/frankban/quicktest"
	"github.com/google/go-cmp/cmp/cmpopts"
	jujuparams "github.com/juju/juju/rpc/params"

	"github.com/canonical/jimm/v3/internal/db"
	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/testutils/jimmtest"
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

	cloud := dbmodel.Cloud{
		Name: "test-cloud",
	}
	err = s.Database.AddCloud(context.Background(), &cloud)
	c.Assert(err, qt.IsNil)

	controller := dbmodel.Controller{
		Name:      "test-controller",
		UUID:      "00000000-0000-0000-0000-0000-0000000000001",
		CloudName: "test-cloud",
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

	cloud := dbmodel.Cloud{
		Name: "test-cloud",
	}
	err = s.Database.AddCloud(context.Background(), &cloud)
	c.Assert(err, qt.IsNil)

	controller := dbmodel.Controller{
		Name:      "test-controller",
		UUID:      "00000000-0000-0000-0000-0000-0000000000001",
		CloudName: "test-cloud",
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
  owner: alice@canonical.com
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
	err := d.UpdateController(context.Background(), &dbmodel.Controller{ID: 1})
	c.Check(err, qt.ErrorMatches, `database not configured`)
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeServerConfiguration)
}

func (s *dbSuite) TestUpdateController(c *qt.C) {
	err := s.Database.Migrate(context.Background(), true)
	c.Assert(err, qt.Equals, nil)

	cloud := dbmodel.Cloud{
		Name: "test-cloud",
		Type: "test-provider",
		Regions: []dbmodel.CloudRegion{{
			Name: "test-region",
		}},
	}
	c.Assert(s.Database.DB.Create(&cloud).Error, qt.IsNil)

	controller := dbmodel.Controller{
		Name:        "test-controller",
		UUID:        "00000000-0000-0000-0000-0000-0000000000001",
		CloudName:   "test-cloud",
		CloudRegion: "test-region",
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
	controller.Addresses = dbmodel.HostPorts{{{Address: jujuparams.Address{Value: "test-address"}, Port: 10}}}

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

	cloud := dbmodel.Cloud{
		Name: "test-cloud",
	}
	err = s.Database.AddCloud(context.Background(), &cloud)
	c.Assert(err, qt.IsNil)

	controller := dbmodel.Controller{
		Name:      "test-controller",
		UUID:      "00000000-0000-0000-0000-0000-0000000000001",
		CloudName: "test-cloud",
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
	c.Assert(err, qt.ErrorMatches, "controller not found")

	err = s.Database.DeleteController(context.Background(), &dbmodel.Controller{})
	c.Assert(err, qt.ErrorMatches, "controller not found")
}

func TestForEachControllerModelUnconfiguredDatabase(t *testing.T) {
	c := qt.New(t)

	var d db.Database
	err := d.ForEachControllerModel(context.Background(), nil, nil)
	c.Check(err, qt.ErrorMatches, `database not configured`)
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeServerConfiguration)
}

const testForEachControllerModelEnv = `clouds:
- name: test
  type: test
  regions:
  - name: test-region
cloud-credentials:
- name: test-cred
  cloud: test
  owner: alice@canonical.com
  type: empty
controllers:
- name: test
  uuid: 00000001-0000-0000-0000-000000000001
  cloud: test
  region: test-region
- name: test-2
  uuid: 00000001-0000-0000-0000-000000000002
  cloud: test
  region: test-region
models:
- name: test-1
  owner: alice@canonical.com
  uuid: 00000002-0000-0000-0000-000000000001
  controller: test
  cloud: test
  region: test-region
  cloud-credential: test-cred
- name: test-2
  owner: alice@canonical.com
  uuid: 00000002-0000-0000-0000-000000000002
  controller: test
  cloud: test
  region: test-region
  cloud-credential: test-cred
- name: test-3
  owner: alice@canonical.com
  uuid: 00000002-0000-0000-0000-000000000003
  controller: test-2
  cloud: test
  region: test-region
  cloud-credential: test-cred
- name: test-4
  owner: alice@canonical.com
  uuid: 00000002-0000-0000-0000-000000000004
  controller: test
  cloud: test
  region: test-region
  cloud-credential: test-cred
`

func (s *dbSuite) TestForEachControllerModel(c *qt.C) {
	ctx := context.Background()
	err := s.Database.Migrate(context.Background(), true)
	c.Assert(err, qt.Equals, nil)

	env := jimmtest.ParseEnvironment(c, testForEachControllerModelEnv)
	env.PopulateDB(c, *s.Database)

	ctl := env.Controller("test").DBObject(c, *s.Database)
	testError := errors.E("test error")
	err = s.Database.ForEachControllerModel(ctx, &ctl, func(_ *dbmodel.Model) error {
		return testError
	})
	c.Check(err, qt.Equals, testError)

	var models []string
	err = s.Database.ForEachControllerModel(ctx, &ctl, func(model *dbmodel.Model) error {
		models = append(models, model.UUID.String)
		return nil
	})
	c.Assert(err, qt.IsNil)
	c.Check(models, qt.DeepEquals, []string{
		"00000002-0000-0000-0000-000000000001",
		"00000002-0000-0000-0000-000000000002",
		"00000002-0000-0000-0000-000000000004",
	})
}

func TestUpsertControllerConfigUnconfiguredDatabase(t *testing.T) {
	c := qt.New(t)

	var d db.Database
	err := d.UpsertControllerConfig(context.Background(), &dbmodel.ControllerConfig{})
	c.Check(err, qt.ErrorMatches, `database not configured`)
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeServerConfiguration)
}

func (s *dbSuite) TestControllerConfig(c *qt.C) {
	ctx := context.Background()
	err := s.Database.Migrate(context.Background(), true)
	c.Assert(err, qt.Equals, nil)

	config := dbmodel.ControllerConfig{
		Name: "jimm",
		Config: map[string]interface{}{
			"key1": "value1",
			"key2": "value2",
		},
	}
	err = s.Database.UpsertControllerConfig(ctx, &config)
	c.Assert(err, qt.IsNil)

	config1 := dbmodel.ControllerConfig{
		Name: "jimm",
	}
	err = s.Database.GetControllerConfig(ctx, &config1)
	c.Assert(err, qt.IsNil)
	c.Assert(config1, qt.DeepEquals, config)

	config2 := config1
	config2.Config = map[string]interface{}{
		"key1": "value1.1",
		"key2": "value2.1",
		"key3": "value3",
	}
	err = s.Database.UpsertControllerConfig(ctx, &config2)
	c.Assert(err, qt.IsNil)

	err = s.Database.GetControllerConfig(ctx, &config1)
	c.Assert(err, qt.IsNil)
	c.Assert(config1, qt.DeepEquals, config2)

	config3 := dbmodel.ControllerConfig{
		Name: "unknown",
	}
	err = s.Database.GetControllerConfig(ctx, &config3)
	c.Assert(err, qt.ErrorMatches, "controller config not found")
}

func TestGetControllerConfigUnconfiguredDatabase(t *testing.T) {
	c := qt.New(t)

	var d db.Database
	err := d.GetControllerConfig(context.Background(), &dbmodel.ControllerConfig{})
	c.Check(err, qt.ErrorMatches, `database not configured`)
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeServerConfiguration)
}
