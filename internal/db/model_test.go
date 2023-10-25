// Copyright 2020 Canonical Ltd.

package db_test

import (
	"context"
	"database/sql"
	"testing"

	qt "github.com/frankban/quicktest"
	"gorm.io/gorm"

	"github.com/canonical/jimm/internal/db"
	"github.com/canonical/jimm/internal/dbmodel"
	"github.com/canonical/jimm/internal/errors"
	"github.com/canonical/jimm/internal/jimmtest"
)

func TestAddModelUnconfiguredDatabase(t *testing.T) {
	c := qt.New(t)

	var d db.Database
	err := d.AddModel(context.Background(), &dbmodel.Model{})
	c.Check(err, qt.ErrorMatches, `database not configured`)
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeServerConfiguration)
}

func (s *dbSuite) TestAddModel(c *qt.C) {
	err := s.Database.Migrate(context.Background(), true)
	c.Assert(err, qt.Equals, nil)

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

	controller := dbmodel.Controller{
		Name:   "test-controller",
		UUID:   "00000000-0000-0000-0000-0000-0000000000001",
		Models: []dbmodel.Model{},
	}
	err = s.Database.AddController(context.Background(), &controller)
	c.Assert(err, qt.Equals, nil)

	model := dbmodel.Model{
		Name: "test-model-1",
		UUID: sql.NullString{
			String: "00000001-0000-0000-0000-0000-000000000001",
			Valid:  true,
		},
		OwnerUsername:     u.Username,
		ControllerID:      controller.ID,
		CloudRegionID:     cloud.Regions[0].ID,
		CloudCredentialID: cred.ID,
		Type:              "iaas",
		DefaultSeries:     "warty",
		Life:              "alive",
		Status: dbmodel.Status{
			Status: "available",
			Since:  db.Now(),
		},
		SLA: dbmodel.SLA{
			Level: "unsupported",
		},
		Users: []dbmodel.UserModelAccess{{
			Username: u.Username,
			Access:   "admin",
		}},
	}
	m1 := model
	err = s.Database.AddModel(context.Background(), &model)
	c.Assert(err, qt.Equals, nil)

	var dbModel dbmodel.Model
	result := s.Database.DB.Where("uuid = ?", model.UUID).Preload("Users").First(&dbModel)
	c.Assert(result.Error, qt.Equals, nil)
	c.Assert(dbModel, qt.DeepEquals, model)

	err = s.Database.AddModel(context.Background(), &m1)
	c.Assert(err, qt.Not(qt.IsNil))
	eError, ok := err.(*errors.Error)
	c.Assert(ok, qt.IsTrue)
	c.Assert(eError.Code, qt.Equals, errors.CodeAlreadyExists)
}

func (s *dbSuite) TestGetModel(c *qt.C) {
	err := s.Database.Migrate(context.Background(), true)
	c.Assert(err, qt.Equals, nil)

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

	controller := dbmodel.Controller{
		Name:   "test-controller",
		UUID:   "00000000-0000-0000-0000-0000-0000000000001",
		Models: []dbmodel.Model{},
	}
	err = s.Database.AddController(context.Background(), &controller)
	c.Assert(err, qt.Equals, nil)

	model := dbmodel.Model{
		Name: "test-model-1",
		UUID: sql.NullString{
			String: "00000001-0000-0000-0000-0000-000000000001",
			Valid:  true,
		},
		OwnerUsername:     u.Username,
		Owner:             u,
		ControllerID:      controller.ID,
		Controller:        controller,
		CloudRegionID:     cloud.Regions[0].ID,
		CloudRegion:       cloud.Regions[0],
		CloudCredentialID: cred.ID,
		CloudCredential:   cred,
		Type:              "iaas",
		DefaultSeries:     "warty",
		Life:              "alive",
		Status: dbmodel.Status{
			Status: "available",
			Since:  db.Now(),
		},
		SLA: dbmodel.SLA{
			Level: "unsupported",
		},
		Users: []dbmodel.UserModelAccess{{
			Username: u.Username,
			User:     u,
			Access:   "admin",
		}},
	}
	model.CloudCredential.Cloud = dbmodel.Cloud{}
	model.CloudCredential.Owner = dbmodel.User{}
	err = s.Database.AddModel(context.Background(), &model)
	c.Assert(err, qt.Equals, nil)

	dbModel := dbmodel.Model{
		UUID: model.UUID,
	}
	err = s.Database.GetModel(context.Background(), &dbModel)
	c.Assert(err, qt.Equals, nil)
	expectModel := model
	expectModel.CloudRegion.Cloud = cloud
	expectModel.CloudRegion.Cloud.Regions = nil
	c.Assert(dbModel, jimmtest.DBObjectEquals, expectModel)

	dbModel = dbmodel.Model{
		UUID: sql.NullString{
			String: "no such model",
			Valid:  true,
		},
	}
	err = s.Database.GetModel(context.Background(), &dbModel)
	c.Assert(err, qt.Not(qt.IsNil))
	eError, ok := err.(*errors.Error)
	c.Assert(ok, qt.IsTrue)
	c.Assert(eError.Code, qt.Equals, errors.CodeNotFound)
}

func (s *dbSuite) TestUpdateModel(c *qt.C) {
	err := s.Database.Migrate(context.Background(), true)
	c.Assert(err, qt.Equals, nil)

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

	controller := dbmodel.Controller{
		Name:   "test-controller",
		UUID:   "00000000-0000-0000-0000-0000-0000000000001",
		Models: []dbmodel.Model{},
	}
	err = s.Database.AddController(context.Background(), &controller)
	c.Assert(err, qt.Equals, nil)

	model := dbmodel.Model{
		Name:              "test-model-1",
		OwnerUsername:     u.Username,
		ControllerID:      controller.ID,
		CloudRegionID:     cloud.Regions[0].ID,
		CloudCredentialID: cred.ID,
		Type:              "iaas",
		DefaultSeries:     "warty",
		Life:              "alive",
		Status: dbmodel.Status{
			Status: "available",
			Since:  db.Now(),
		},
		SLA: dbmodel.SLA{
			Level: "unsupported",
		},
		Users: []dbmodel.UserModelAccess{{
			Username: u.Username,
			Access:   "admin",
		}},
	}
	err = s.Database.AddModel(context.Background(), &model)
	c.Assert(err, qt.Equals, nil)

	model.UUID = sql.NullString{
		String: "00000001-0000-0000-0000-0000-000000000001",
		Valid:  true,
	}
	err = s.Database.UpdateModel(context.Background(), &model)
	c.Assert(err, qt.Equals, nil)

	var dbModel dbmodel.Model
	result := s.Database.DB.Where("uuid = ?", model.UUID).Preload("Users").First(&dbModel)
	c.Assert(result.Error, qt.Equals, nil)
	c.Assert(dbModel, qt.DeepEquals, model)
}

func TestDeleteModelUnconfiguredDatabase(t *testing.T) {
	c := qt.New(t)

	var d db.Database
	err := d.DeleteModel(context.Background(), &dbmodel.Model{})
	c.Check(err, qt.ErrorMatches, `database not configured`)
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeServerConfiguration)
}

func (s *dbSuite) TestDeleteModel(c *qt.C) {
	err := s.Database.Migrate(context.Background(), true)
	c.Assert(err, qt.Equals, nil)

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

	controller := dbmodel.Controller{
		Name:   "test-controller",
		UUID:   "00000000-0000-0000-0000-0000-0000000000001",
		Models: []dbmodel.Model{},
	}
	err = s.Database.AddController(context.Background(), &controller)
	c.Assert(err, qt.Equals, nil)

	model := dbmodel.Model{
		Name:              "test-model-1",
		OwnerUsername:     u.Username,
		ControllerID:      controller.ID,
		Controller:        controller,
		CloudRegionID:     cloud.Regions[0].ID,
		CloudCredentialID: cred.ID,
		Type:              "iaas",
		DefaultSeries:     "warty",
		Life:              "alive",
		Status: dbmodel.Status{
			Status: "available",
			Since:  db.Now(),
		},
		SLA: dbmodel.SLA{
			Level: "unsupported",
		},
		Users: []dbmodel.UserModelAccess{{
			Username: u.Username,
			User:     u,
			Access:   "admin",
		}},
	}

	// model does not exist
	err = s.Database.DeleteModel(context.Background(), &model)
	c.Assert(err, qt.IsNil)

	err = s.Database.AddModel(context.Background(), &model)
	c.Assert(err, qt.Equals, nil)

	model.UUID = sql.NullString{
		String: "00000001-0000-0000-0000-0000-000000000001",
		Valid:  true,
	}
	err = s.Database.DeleteModel(context.Background(), &model)
	c.Assert(err, qt.Equals, nil)

	var dbModel dbmodel.Model
	result := s.Database.DB.Where("uuid = ?", model.UUID).Preload("Users").First(&dbModel)
	c.Assert(result.Error, qt.Equals, gorm.ErrRecordNotFound)
}

func (s *dbSuite) TestGetModelsUsingCredential(c *qt.C) {
	err := s.Database.Migrate(context.Background(), true)
	c.Assert(err, qt.Equals, nil)

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

	cred1 := dbmodel.CloudCredential{
		Name:     "test-cred-1",
		Cloud:    cloud,
		Owner:    u,
		AuthType: "empty",
	}
	c.Assert(s.Database.DB.Create(&cred1).Error, qt.IsNil)

	cred2 := dbmodel.CloudCredential{
		Name:     "test-cred-2",
		Cloud:    cloud,
		Owner:    u,
		AuthType: "empty",
	}
	c.Assert(s.Database.DB.Create(&cred2).Error, qt.IsNil)

	controller := dbmodel.Controller{
		Name: "test-controller",
		UUID: "00000000-0000-0000-0000-0000-0000000000001",
	}
	err = s.Database.AddController(context.Background(), &controller)
	c.Assert(err, qt.Equals, nil)

	model1 := dbmodel.Model{
		Name: "test-model-1",
		UUID: sql.NullString{
			String: "00000001-0000-0000-0000-0000-000000000001",
			Valid:  true,
		},
		OwnerUsername:     u.Username,
		ControllerID:      controller.ID,
		CloudRegionID:     cloud.Regions[0].ID,
		CloudCredentialID: cred1.ID,
		Type:              "iaas",
		DefaultSeries:     "warty",
		Life:              "alive",
		Status: dbmodel.Status{
			Status: "available",
			Since:  db.Now(),
		},
		SLA: dbmodel.SLA{
			Level: "unsupported",
		},
		Users: []dbmodel.UserModelAccess{{
			Username: u.Username,
			Access:   "admin",
		}},
	}
	err = s.Database.AddModel(context.Background(), &model1)
	c.Assert(err, qt.Equals, nil)

	model2 := dbmodel.Model{
		Name: "test-model-2",
		UUID: sql.NullString{
			String: "00000001-0000-0000-0000-0000-000000000002",
			Valid:  true,
		},
		OwnerUsername:     u.Username,
		ControllerID:      controller.ID,
		CloudRegionID:     cloud.Regions[0].ID,
		CloudCredentialID: cred2.ID,
		Type:              "iaas",
		DefaultSeries:     "warty",
		Life:              "alive",
		Status: dbmodel.Status{
			Status: "available",
			Since:  db.Now(),
		},
		SLA: dbmodel.SLA{
			Level: "unsupported",
		},
		Users: []dbmodel.UserModelAccess{{
			Username: u.Username,
			Access:   "admin",
		}},
	}
	err = s.Database.AddModel(context.Background(), &model2)
	c.Assert(err, qt.Equals, nil)

	models, err := s.Database.GetModelsUsingCredential(context.Background(), cred1.ID)
	c.Assert(err, qt.Equals, nil)
	c.Assert(models, qt.DeepEquals, []dbmodel.Model{{
		ID:        model1.ID,
		CreatedAt: model1.CreatedAt,
		UpdatedAt: model1.UpdatedAt,
		Name:      "test-model-1",
		UUID: sql.NullString{
			String: "00000001-0000-0000-0000-0000-000000000001",
			Valid:  true,
		},
		OwnerUsername:     u.Username,
		ControllerID:      controller.ID,
		Controller:        controller,
		CloudRegionID:     cloud.Regions[0].ID,
		CloudCredentialID: cred1.ID,
		Type:              "iaas",
		DefaultSeries:     "warty",
		Life:              "alive",
		Status:            model1.Status,
		SLA: dbmodel.SLA{
			Level: "unsupported",
		},
	}})

	models, err = s.Database.GetModelsUsingCredential(context.Background(), 0)
	c.Assert(err, qt.IsNil)
	c.Assert(models, qt.HasLen, 0)
}

func TestUpdateUserModelAccessUnconfiguredDatabase(t *testing.T) {
	c := qt.New(t)

	var d db.Database
	err := d.UpdateUserModelAccess(context.Background(), &dbmodel.UserModelAccess{})
	c.Check(err, qt.ErrorMatches, `database not configured`)
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeServerConfiguration)
}

const testUpdateUserModelAccessEnv = `clouds:
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
- name: test
  uuid: 00000001-0000-0000-0000-000000000001
  cloud: test
  region: test-region
models:
- name: test
  uuid: 00000002-0000-0000-0000-000000000001
  owner: alice@external
  cloud: test
  region: test-region
  cloud-credential: test-cred
  controller: test
  users:
  - user: alice@external
    access: admin
  - user: bob@external
    access: write
`

func (s *dbSuite) TestUpdateUserModelAccess(c *qt.C) {
	ctx := context.Background()
	err := s.Database.Migrate(context.Background(), true)
	c.Assert(err, qt.Equals, nil)

	env := jimmtest.ParseEnvironment(c, testUpdateUserModelAccessEnv)
	env.PopulateDB(c, *s.Database)

	m := dbmodel.Model{
		UUID: sql.NullString{
			String: "00000002-0000-0000-0000-000000000001",
			Valid:  true,
		},
	}
	err = s.Database.GetModel(ctx, &m)
	c.Assert(err, qt.IsNil)

	charlie := env.User("charlie@external").DBObject(c, *s.Database)

	// Add a new user
	err = s.Database.UpdateUserModelAccess(ctx, &dbmodel.UserModelAccess{
		User:   charlie,
		Model_: m,
		Access: "read",
	})
	c.Assert(err, qt.Equals, nil)
	err = s.Database.GetModel(ctx, &m)
	c.Check(m.Users, jimmtest.DBObjectEquals, []dbmodel.UserModelAccess{{
		User: dbmodel.User{
			Username:         "alice@external",
			ControllerAccess: "add-model",
		},
		Access: "admin",
	}, {
		User: dbmodel.User{
			Username:         "bob@external",
			ControllerAccess: "add-model",
		},
		Access: "write",
	}, {
		User: dbmodel.User{
			Username:         "charlie@external",
			ControllerAccess: "add-model",
		},
		Access: "read",
	}})

	// Update an existing user
	uma := m.Users[1]
	uma.Access = "read"
	err = s.Database.UpdateUserModelAccess(ctx, &uma)
	c.Assert(err, qt.Equals, nil)
	err = s.Database.GetModel(ctx, &m)
	c.Check(m.Users, jimmtest.DBObjectEquals, []dbmodel.UserModelAccess{{
		User: dbmodel.User{
			Username:         "alice@external",
			ControllerAccess: "add-model",
		},
		Access: "admin",
	}, {
		User: dbmodel.User{
			Username:         "bob@external",
			ControllerAccess: "add-model",
		},
		Access: "read",
	}, {
		User: dbmodel.User{
			Username:         "charlie@external",
			ControllerAccess: "add-model",
		},
		Access: "read",
	}})

	// Remove a user
	uma = m.Users[1]
	uma.Access = ""
	err = s.Database.UpdateUserModelAccess(ctx, &uma)
	c.Assert(err, qt.Equals, nil)
	err = s.Database.GetModel(ctx, &m)
	c.Check(m.Users, jimmtest.DBObjectEquals, []dbmodel.UserModelAccess{{
		User: dbmodel.User{
			Username:         "alice@external",
			ControllerAccess: "add-model",
		},
		Access: "admin",
	}, {
		User: dbmodel.User{
			Username:         "charlie@external",
			ControllerAccess: "add-model",
		},
		Access: "read",
	}})
}

func TestForEachModelUnconfiguredDatabase(t *testing.T) {
	c := qt.New(t)

	var d db.Database
	err := d.ForEachModel(context.Background(), nil)
	c.Check(err, qt.ErrorMatches, `database not configured`)
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeServerConfiguration)
}

const testForEachModelEnv = `clouds:
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
- name: test
  uuid: 00000001-0000-0000-0000-000000000001
  cloud: test
  region: test-region
models:
- name: test-1
  uuid: 00000002-0000-0000-0000-000000000001
  owner: alice@external
  cloud: test
  region: test-region
  cloud-credential: test-cred
  controller: test
  users:
  - user: alice@external
    access: admin
  - user: bob@external
    access: write
- name: test-2
  uuid: 00000002-0000-0000-0000-000000000002
  owner: bob@external
  cloud: test
  region: test-region
  cloud-credential: test-cred
  controller: test
  users:
  - user: bob@external
    access: admin
- name: test-3
  uuid: 00000002-0000-0000-0000-000000000003
  owner: bob@external
  cloud: test
  region: test-region
  cloud-credential: test-cred
  controller: test
  users:
  - user: bob@external
    access: admin
`

func (s *dbSuite) TestForEachModel(c *qt.C) {
	ctx := context.Background()
	err := s.Database.Migrate(context.Background(), true)
	c.Assert(err, qt.Equals, nil)

	env := jimmtest.ParseEnvironment(c, testForEachModelEnv)
	env.PopulateDB(c, *s.Database)

	testError := errors.E("test error")
	err = s.Database.ForEachModel(ctx, func(m *dbmodel.Model) error {
		return testError
	})
	c.Check(err, qt.Equals, testError)

	var models []string
	err = s.Database.ForEachModel(ctx, func(m *dbmodel.Model) error {
		models = append(models, m.UUID.String)
		return nil
	})
	c.Assert(err, qt.IsNil)
	c.Check(models, qt.DeepEquals, []string{
		"00000002-0000-0000-0000-000000000001",
		"00000002-0000-0000-0000-000000000002",
		"00000002-0000-0000-0000-000000000003",
	})
}

func TestGetApplicationUnconfiguredDatabase(t *testing.T) {
	c := qt.New(t)

	var d db.Database
	err := d.GetApplication(context.Background(), nil)
	c.Check(err, qt.ErrorMatches, `database not configured`)
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeServerConfiguration)
}

const testGetApplicationEnv = `clouds:
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
- name: test
  uuid: 00000001-0000-0000-0000-000000000001
  cloud: test
  region: test-region
models:
- name: test-1
  uuid: 00000002-0000-0000-0000-000000000001
  owner: alice@external
  cloud: test
  region: test-region
  cloud-credential: test-cred
  controller: test
  applications:
  - name: app-1
    charm-url: cs:app
  users:
  - user: alice@external
    access: admin
  - user: bob@external
    access: write
`

func (s *dbSuite) TestGetApplication(c *qt.C) {
	ctx := context.Background()
	err := s.Database.GetApplication(ctx, nil)
	c.Check(err, qt.ErrorMatches, `upgrade in progress`)
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeUpgradeInProgress)

	err = s.Database.Migrate(ctx, true)
	c.Assert(err, qt.Equals, nil)

	env := jimmtest.ParseEnvironment(c, testGetApplicationEnv)
	env.PopulateDB(c, *s.Database)

	app := dbmodel.Application{
		ModelID: env.Model("alice@external", "test-1").DBObject(c, *s.Database).ID,
		Name:    "app-1",
	}

	err = s.Database.GetApplication(ctx, &app)
	c.Assert(err, qt.IsNil)

	app2 := dbmodel.Application{
		ModelID: app.ModelID,
		Name:    "app-2",
	}

	err = s.Database.GetApplication(ctx, &app2)
	c.Check(err, qt.ErrorMatches, `application not found`)
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeNotFound)
}

func TestDeleteApplicationUnconfiguredDatabase(t *testing.T) {
	c := qt.New(t)

	var d db.Database
	err := d.DeleteApplication(context.Background(), nil)
	c.Check(err, qt.ErrorMatches, `database not configured`)
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeServerConfiguration)
}

func (s *dbSuite) TestDeleteApplication(c *qt.C) {
	ctx := context.Background()
	err := s.Database.DeleteApplication(ctx, nil)
	c.Check(err, qt.ErrorMatches, `upgrade in progress`)
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeUpgradeInProgress)

	err = s.Database.Migrate(ctx, true)
	c.Assert(err, qt.Equals, nil)

	env := jimmtest.ParseEnvironment(c, testGetApplicationEnv)
	env.PopulateDB(c, *s.Database)

	app := dbmodel.Application{
		ModelID: env.Model("alice@external", "test-1").DBObject(c, *s.Database).ID,
		Name:    "app-1",
	}
	err = s.Database.DeleteApplication(ctx, &app)
	c.Assert(err, qt.IsNil)
	err = s.Database.GetApplication(ctx, &app)
	c.Check(err, qt.ErrorMatches, `application not found`)
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeNotFound)

	err = s.Database.DeleteApplication(ctx, &app)
	c.Assert(err, qt.IsNil)
	err = s.Database.GetApplication(ctx, &app)
	c.Check(err, qt.ErrorMatches, `application not found`)
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeNotFound)
}

func TestUpdateApplicationUnconfiguredDatabase(t *testing.T) {
	c := qt.New(t)

	var d db.Database
	err := d.UpdateApplication(context.Background(), nil)
	c.Check(err, qt.ErrorMatches, `database not configured`)
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeServerConfiguration)
}

func (s *dbSuite) TestUpdateApplication(c *qt.C) {
	ctx := context.Background()
	err := s.Database.UpdateApplication(ctx, nil)
	c.Check(err, qt.ErrorMatches, `upgrade in progress`)
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeUpgradeInProgress)

	err = s.Database.Migrate(ctx, true)
	c.Assert(err, qt.Equals, nil)

	env := jimmtest.ParseEnvironment(c, testGetApplicationEnv)
	env.PopulateDB(c, *s.Database)

	app := dbmodel.Application{
		ModelID: env.Model("alice@external", "test-1").DBObject(c, *s.Database).ID,
		Name:    "app-2",
		Life:    "starting",
	}
	err = s.Database.UpdateApplication(ctx, &app)
	c.Assert(err, qt.IsNil)

	app2 := dbmodel.Application{
		ModelID: app.ModelID,
		Name:    "app-2",
	}
	err = s.Database.GetApplication(ctx, &app2)
	c.Assert(err, qt.IsNil)
	c.Check(app2, jimmtest.DBObjectEquals, app)

	app2.Life = "alive"
	err = s.Database.UpdateApplication(ctx, &app2)
	c.Assert(err, qt.IsNil)

	app3 := dbmodel.Application{
		ModelID: app.ModelID,
		Name:    "app-2",
	}
	err = s.Database.GetApplication(ctx, &app3)
	c.Assert(err, qt.IsNil)
	c.Check(app3, jimmtest.DBObjectEquals, app2)
}

func TestGetMachineUnconfiguredDatabase(t *testing.T) {
	c := qt.New(t)

	var d db.Database
	err := d.GetMachine(context.Background(), nil)
	c.Check(err, qt.ErrorMatches, `database not configured`)
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeServerConfiguration)
}

const testGetMachineEnv = `clouds:
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
- name: test
  uuid: 00000001-0000-0000-0000-000000000001
  cloud: test
  region: test-region
models:
- name: test-1
  uuid: 00000002-0000-0000-0000-000000000001
  owner: alice@external
  cloud: test
  region: test-region
  cloud-credential: test-cred
  controller: test
  machines:
  - id: "0"
    display-name: Test Machine
  users:
  - user: alice@external
    access: admin
  - user: bob@external
    access: write
`

func (s *dbSuite) TestGetMachine(c *qt.C) {
	ctx := context.Background()
	err := s.Database.GetMachine(ctx, nil)
	c.Check(err, qt.ErrorMatches, `upgrade in progress`)
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeUpgradeInProgress)

	err = s.Database.Migrate(ctx, true)
	c.Assert(err, qt.Equals, nil)

	env := jimmtest.ParseEnvironment(c, testGetMachineEnv)
	env.PopulateDB(c, *s.Database)

	m := dbmodel.Machine{
		ModelID:   env.Model("alice@external", "test-1").DBObject(c, *s.Database).ID,
		MachineID: "0",
	}

	err = s.Database.GetMachine(ctx, &m)
	c.Assert(err, qt.IsNil)
	c.Check(m.DisplayName, qt.Equals, "Test Machine")

	m2 := dbmodel.Machine{
		ModelID:   m.ModelID,
		MachineID: "1",
	}

	err = s.Database.GetMachine(ctx, &m2)
	c.Check(err, qt.ErrorMatches, `machine not found`)
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeNotFound)
}

func TestDeleteMachineUnconfiguredDatabase(t *testing.T) {
	c := qt.New(t)

	var d db.Database
	err := d.DeleteMachine(context.Background(), nil)
	c.Check(err, qt.ErrorMatches, `database not configured`)
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeServerConfiguration)
}

func (s *dbSuite) TestDeleteMachine(c *qt.C) {
	ctx := context.Background()
	err := s.Database.DeleteMachine(ctx, nil)
	c.Check(err, qt.ErrorMatches, `upgrade in progress`)
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeUpgradeInProgress)

	err = s.Database.Migrate(ctx, true)
	c.Assert(err, qt.Equals, nil)

	env := jimmtest.ParseEnvironment(c, testGetMachineEnv)
	env.PopulateDB(c, *s.Database)

	m := dbmodel.Machine{
		ModelID:   env.Model("alice@external", "test-1").DBObject(c, *s.Database).ID,
		MachineID: "0",
	}
	err = s.Database.DeleteMachine(ctx, &m)
	c.Assert(err, qt.IsNil)
	err = s.Database.GetMachine(ctx, &m)
	c.Check(err, qt.ErrorMatches, `machine not found`)
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeNotFound)

	err = s.Database.DeleteMachine(ctx, &m)
	c.Assert(err, qt.IsNil)
	err = s.Database.GetMachine(ctx, &m)
	c.Check(err, qt.ErrorMatches, `machine not found`)
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeNotFound)
}

func TestUpdateMachineUnconfiguredDatabase(t *testing.T) {
	c := qt.New(t)

	var d db.Database
	err := d.UpdateMachine(context.Background(), nil)
	c.Check(err, qt.ErrorMatches, `database not configured`)
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeServerConfiguration)
}

func (s *dbSuite) TestUpdateMachine(c *qt.C) {
	ctx := context.Background()
	err := s.Database.UpdateMachine(ctx, nil)
	c.Check(err, qt.ErrorMatches, `upgrade in progress`)
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeUpgradeInProgress)

	err = s.Database.Migrate(ctx, true)
	c.Assert(err, qt.Equals, nil)

	env := jimmtest.ParseEnvironment(c, testGetMachineEnv)
	env.PopulateDB(c, *s.Database)

	m := dbmodel.Machine{
		ModelID:     env.Model("alice@external", "test-1").DBObject(c, *s.Database).ID,
		MachineID:   "1",
		DisplayName: "Machine 1",
	}
	err = s.Database.UpdateMachine(ctx, &m)
	c.Assert(err, qt.IsNil)

	m2 := dbmodel.Machine{
		ModelID:   m.ModelID,
		MachineID: "1",
	}
	err = s.Database.GetMachine(ctx, &m2)
	c.Assert(err, qt.IsNil)
	c.Check(m2, jimmtest.DBObjectEquals, m)

	m2.DisplayName = "Updated"
	err = s.Database.UpdateMachine(ctx, &m2)
	c.Assert(err, qt.IsNil)

	m3 := dbmodel.Machine{
		ModelID:   m.ModelID,
		MachineID: "1",
	}
	err = s.Database.GetMachine(ctx, &m3)
	c.Assert(err, qt.IsNil)
	c.Check(m3, jimmtest.DBObjectEquals, m2)
}

const testGetUnitEnv = `clouds:
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
- name: test
  uuid: 00000001-0000-0000-0000-000000000001
  cloud: test
  region: test-region
models:
- name: test-1
  uuid: 00000002-0000-0000-0000-000000000001
  owner: alice@external
  cloud: test
  region: test-region
  cloud-credential: test-cred
  controller: test
  applications:
  - name: app-1
  machines:
  - id: "0"
  units:
  - name: app-1/0
    application: app-1
    machine-id: "0"
    life: starting
  users:
  - user: alice@external
    access: admin
  - user: bob@external
    access: write
`

func (s *dbSuite) TestGetUnit(c *qt.C) {
	ctx := context.Background()
	err := s.Database.GetUnit(ctx, nil)
	c.Check(err, qt.ErrorMatches, `upgrade in progress`)
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeUpgradeInProgress)

	err = s.Database.Migrate(ctx, true)
	c.Assert(err, qt.Equals, nil)

	env := jimmtest.ParseEnvironment(c, testGetUnitEnv)
	env.PopulateDB(c, *s.Database)

	u := dbmodel.Unit{
		ModelID: env.Model("alice@external", "test-1").DBObject(c, *s.Database).ID,
		Name:    "app-1/0",
	}

	err = s.Database.GetUnit(ctx, &u)
	c.Assert(err, qt.IsNil)
	c.Check(u.Life, qt.Equals, "starting")

	u2 := dbmodel.Unit{
		ModelID: u.ModelID,
		Name:    "app-1/1",
	}

	err = s.Database.GetUnit(ctx, &u2)
	c.Check(err, qt.ErrorMatches, `unit not found`)
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeNotFound)
}

func TestDeleteUnitUnconfiguredDatabase(t *testing.T) {
	c := qt.New(t)

	var d db.Database
	err := d.DeleteUnit(context.Background(), nil)
	c.Check(err, qt.ErrorMatches, `database not configured`)
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeServerConfiguration)
}

func (s *dbSuite) TestDeleteUnit(c *qt.C) {
	ctx := context.Background()
	err := s.Database.DeleteUnit(ctx, nil)
	c.Check(err, qt.ErrorMatches, `upgrade in progress`)
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeUpgradeInProgress)

	err = s.Database.Migrate(ctx, true)
	c.Assert(err, qt.Equals, nil)

	env := jimmtest.ParseEnvironment(c, testGetUnitEnv)
	env.PopulateDB(c, *s.Database)

	u := dbmodel.Unit{
		ModelID: env.Model("alice@external", "test-1").DBObject(c, *s.Database).ID,
		Name:    "app-1/0",
	}
	err = s.Database.DeleteUnit(ctx, &u)
	c.Assert(err, qt.IsNil)
	err = s.Database.GetUnit(ctx, &u)
	c.Check(err, qt.ErrorMatches, `unit not found`)
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeNotFound)

	err = s.Database.DeleteUnit(ctx, &u)
	c.Assert(err, qt.IsNil)
	err = s.Database.GetUnit(ctx, &u)
	c.Check(err, qt.ErrorMatches, `unit not found`)
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeNotFound)
}

func TestUpdateUnitUnconfiguredDatabase(t *testing.T) {
	c := qt.New(t)

	var d db.Database
	err := d.UpdateUnit(context.Background(), nil)
	c.Check(err, qt.ErrorMatches, `database not configured`)
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeServerConfiguration)
}

func (s *dbSuite) TestUpdateUnit(c *qt.C) {
	ctx := context.Background()
	err := s.Database.UpdateUnit(ctx, nil)
	c.Check(err, qt.ErrorMatches, `upgrade in progress`)
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeUpgradeInProgress)

	err = s.Database.Migrate(ctx, true)
	c.Assert(err, qt.Equals, nil)

	env := jimmtest.ParseEnvironment(c, testGetUnitEnv)
	env.PopulateDB(c, *s.Database)

	u := dbmodel.Unit{
		ModelID:         env.Model("alice@external", "test-1").DBObject(c, *s.Database).ID,
		ApplicationName: "app-1",
		MachineID:       "0",
		Name:            "app-1/1",
		Life:            "starting",
	}
	err = s.Database.UpdateUnit(ctx, &u)
	c.Assert(err, qt.IsNil)

	u2 := dbmodel.Unit{
		ModelID: u.ModelID,
		Name:    "app-1/1",
	}
	err = s.Database.GetUnit(ctx, &u2)
	c.Assert(err, qt.IsNil)
	c.Check(u2, jimmtest.DBObjectEquals, u)

	u2.Life = "alive"
	err = s.Database.UpdateUnit(ctx, &u2)
	c.Assert(err, qt.IsNil)

	u3 := dbmodel.Unit{
		ModelID: u.ModelID,
		Name:    "app-1/1",
	}
	err = s.Database.GetUnit(ctx, &u3)
	c.Assert(err, qt.IsNil)
	c.Check(u3, jimmtest.DBObjectEquals, u2)
}
