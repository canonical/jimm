// Copyright 2024 Canonical.

package db_test

import (
	"context"
	"database/sql"
	"sort"
	"testing"
	"time"

	qt "github.com/frankban/quicktest"
	"github.com/juju/juju/state"
	"gorm.io/gorm"

	"github.com/canonical/jimm/v3/internal/db"
	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/testutils/jimmtest"
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

	u, err := dbmodel.NewIdentity("bob@canonical.com")
	c.Assert(err, qt.IsNil)
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
		Owner:    *u,
		AuthType: "empty",
	}
	c.Assert(s.Database.DB.Create(&cred).Error, qt.IsNil)

	controller := dbmodel.Controller{
		Name:        "test-controller",
		UUID:        "00000000-0000-0000-0000-0000-0000000000001",
		CloudName:   "test-cloud",
		CloudRegion: "test-region",
	}
	err = s.Database.AddController(context.Background(), &controller)
	c.Assert(err, qt.Equals, nil)

	model := dbmodel.Model{
		Name: "test-model-1",
		UUID: sql.NullString{
			String: "00000001-0000-0000-0000-0000-000000000001",
			Valid:  true,
		},
		OwnerIdentityName: u.Name,
		ControllerID:      controller.ID,
		CloudRegionID:     cloud.Regions[0].ID,
		CloudCredentialID: cred.ID,
		Type:              "iaas",
		DefaultSeries:     "warty",
		Life:              state.Alive.String(),
		Status: dbmodel.Status{
			Status: "available",
			Since:  db.Now(),
		},
		SLA: dbmodel.SLA{
			Level: "unsupported",
		},
	}
	m1 := model
	err = s.Database.AddModel(context.Background(), &model)
	c.Assert(err, qt.Equals, nil)

	var dbModel dbmodel.Model
	result := s.Database.DB.Where("uuid = ?", model.UUID).First(&dbModel)
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

	u, err := dbmodel.NewIdentity("bob@canonical.com")
	c.Assert(err, qt.IsNil)
	c.Assert(s.Database.DB.Create(u).Error, qt.IsNil)

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
		Owner:    *u,
		AuthType: "empty",
	}
	c.Assert(s.Database.DB.Create(&cred).Error, qt.IsNil)

	controller := dbmodel.Controller{
		Name:        "test-controller",
		UUID:        "00000000-0000-0000-0000-0000-0000000000001",
		CloudName:   "test-cloud",
		CloudRegion: "test-region",
	}
	err = s.Database.AddController(context.Background(), &controller)
	c.Assert(err, qt.Equals, nil)

	model := dbmodel.Model{
		Name: "test-model-1",
		UUID: sql.NullString{
			String: "00000001-0000-0000-0000-0000-000000000001",
			Valid:  true,
		},
		OwnerIdentityName: u.Name,
		Owner:             *u,
		ControllerID:      controller.ID,
		Controller:        controller,
		CloudRegionID:     cloud.Regions[0].ID,
		CloudRegion:       cloud.Regions[0],
		CloudCredentialID: cred.ID,
		CloudCredential:   cred,
		Type:              "iaas",
		DefaultSeries:     "warty",
		Life:              state.Alive.String(),
		Status: dbmodel.Status{
			Status: "available",
			Since:  db.Now(),
		},
		SLA: dbmodel.SLA{
			Level: "unsupported",
		},
	}
	model.CloudCredential.Cloud = dbmodel.Cloud{}
	// We don't care about the cloud credential owner when
	// loading a model, as we just use the credential to deploy
	// applications. Hence, we don't use NewIdentity() here
	model.CloudCredential.Owner = dbmodel.Identity{}
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

	dbModel = dbmodel.Model{
		Name:              model.Name,
		OwnerIdentityName: model.OwnerIdentityName,
	}
	err = s.Database.GetModel(context.Background(), &dbModel)
	c.Assert(err, qt.IsNil)
	expectModel = model
	expectModel.CloudRegion.Cloud = cloud
	expectModel.CloudRegion.Cloud.Regions = nil
	c.Assert(dbModel, jimmtest.DBObjectEquals, expectModel)
}

func (s *dbSuite) TestUpdateModel(c *qt.C) {
	err := s.Database.Migrate(context.Background(), true)
	c.Assert(err, qt.Equals, nil)

	i, err := dbmodel.NewIdentity("bob@canonical.com")
	c.Assert(err, qt.IsNil)
	c.Assert(s.Database.DB.Create(i).Error, qt.IsNil)

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
		Owner:    *i,
		AuthType: "empty",
	}
	c.Assert(s.Database.DB.Create(&cred).Error, qt.IsNil)

	controller := dbmodel.Controller{
		Name:        "test-controller",
		UUID:        "00000000-0000-0000-0000-0000-0000000000001",
		CloudName:   "test-cloud",
		CloudRegion: "test-region",
	}
	err = s.Database.AddController(context.Background(), &controller)
	c.Assert(err, qt.Equals, nil)

	model := dbmodel.Model{
		Name:              "test-model-1",
		OwnerIdentityName: i.Name,
		ControllerID:      controller.ID,
		CloudRegionID:     cloud.Regions[0].ID,
		CloudCredentialID: cred.ID,
		Type:              "iaas",
		DefaultSeries:     "warty",
		Life:              state.Alive.String(),
		Status: dbmodel.Status{
			Status: "available",
			Since:  db.Now(),
		},
		SLA: dbmodel.SLA{
			Level: "unsupported",
		},
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
	result := s.Database.DB.Where("uuid = ?", model.UUID).First(&dbModel)
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

	i, err := dbmodel.NewIdentity("bob@canonical.com")
	c.Assert(err, qt.IsNil)
	c.Assert(s.Database.DB.Create(i).Error, qt.IsNil)

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
		Owner:    *i,
		AuthType: "empty",
	}
	c.Assert(s.Database.DB.Create(&cred).Error, qt.IsNil)

	controller := dbmodel.Controller{
		Name:        "test-controller",
		UUID:        "00000000-0000-0000-0000-0000-0000000000001",
		CloudName:   "test-cloud",
		CloudRegion: "test-region",
	}
	err = s.Database.AddController(context.Background(), &controller)
	c.Assert(err, qt.Equals, nil)

	model := dbmodel.Model{
		Name:              "test-model-1",
		OwnerIdentityName: i.Name,
		ControllerID:      controller.ID,
		Controller:        controller,
		CloudRegionID:     cloud.Regions[0].ID,
		CloudCredentialID: cred.ID,
		Type:              "iaas",
		DefaultSeries:     "warty",
		Life:              state.Alive.String(),
		Status: dbmodel.Status{
			Status: "available",
			Since:  db.Now(),
		},
		SLA: dbmodel.SLA{
			Level: "unsupported",
		},
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
	result := s.Database.DB.Where("uuid = ?", model.UUID).First(&dbModel)
	c.Assert(result.Error, qt.Equals, gorm.ErrRecordNotFound)
}

func (s *dbSuite) TestGetModelsUsingCredential(c *qt.C) {
	err := s.Database.Migrate(context.Background(), true)
	c.Assert(err, qt.Equals, nil)

	i, err := dbmodel.NewIdentity("bob@canonical.com")
	c.Assert(err, qt.IsNil)
	c.Assert(s.Database.DB.Create(i).Error, qt.IsNil)

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
		Owner:    *i,
		AuthType: "empty",
	}
	c.Assert(s.Database.DB.Create(&cred1).Error, qt.IsNil)

	cred2 := dbmodel.CloudCredential{
		Name:     "test-cred-2",
		Cloud:    cloud,
		Owner:    *i,
		AuthType: "empty",
	}
	c.Assert(s.Database.DB.Create(&cred2).Error, qt.IsNil)

	controller := dbmodel.Controller{
		Name:        "test-controller",
		UUID:        "00000000-0000-0000-0000-0000-0000000000001",
		CloudName:   "test-cloud",
		CloudRegion: "test-region",
	}
	err = s.Database.AddController(context.Background(), &controller)
	c.Assert(err, qt.Equals, nil)

	model1 := dbmodel.Model{
		Name: "test-model-1",
		UUID: sql.NullString{
			String: "00000001-0000-0000-0000-0000-000000000001",
			Valid:  true,
		},
		OwnerIdentityName: i.Name,
		ControllerID:      controller.ID,
		CloudRegionID:     cloud.Regions[0].ID,
		CloudCredentialID: cred1.ID,
		Type:              "iaas",
		DefaultSeries:     "warty",
		Life:              state.Alive.String(),
		Status: dbmodel.Status{
			Status: "available",
			Since:  db.Now(),
		},
		SLA: dbmodel.SLA{
			Level: "unsupported",
		},
	}
	err = s.Database.AddModel(context.Background(), &model1)
	c.Assert(err, qt.Equals, nil)

	model2 := dbmodel.Model{
		Name: "test-model-2",
		UUID: sql.NullString{
			String: "00000001-0000-0000-0000-0000-000000000002",
			Valid:  true,
		},
		OwnerIdentityName: i.Name,
		ControllerID:      controller.ID,
		CloudRegionID:     cloud.Regions[0].ID,
		CloudCredentialID: cred2.ID,
		Type:              "iaas",
		DefaultSeries:     "warty",
		Life:              state.Alive.String(),
		Status: dbmodel.Status{
			Status: "available",
			Since:  db.Now(),
		},
		SLA: dbmodel.SLA{
			Level: "unsupported",
		},
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
		OwnerIdentityName: i.Name,
		ControllerID:      controller.ID,
		Controller:        controller,
		CloudRegionID:     cloud.Regions[0].ID,
		CloudCredentialID: cred1.ID,
		Type:              "iaas",
		DefaultSeries:     "warty",
		Life:              state.Alive.String(),
		Status:            model1.Status,
		SLA: dbmodel.SLA{
			Level: "unsupported",
		},
	}})

	models, err = s.Database.GetModelsUsingCredential(context.Background(), 0)
	c.Assert(err, qt.IsNil)
	c.Assert(models, qt.HasLen, 0)
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
  owner: alice@canonical.com
  type: empty
controllers:
- name: test
  uuid: 00000001-0000-0000-0000-000000000001
  cloud: test
  region: test-region
models:
- name: test-1
  uuid: 00000002-0000-0000-0000-000000000001
  owner: alice@canonical.com
  cloud: test
  region: test-region
  cloud-credential: test-cred
  controller: test
  users:
  - user: alice@canonical.com
    access: admin
  - user: bob@canonical.com
    access: write
- name: test-2
  uuid: 00000002-0000-0000-0000-000000000002
  owner: bob@canonical.com
  cloud: test
  region: test-region
  cloud-credential: test-cred
  controller: test
  users:
  - user: bob@canonical.com
    access: admin
- name: test-3
  uuid: 00000002-0000-0000-0000-000000000003
  owner: bob@canonical.com
  cloud: test
  region: test-region
  cloud-credential: test-cred
  controller: test
  users:
  - user: bob@canonical.com
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

const testGetModelsByUUIDEnv = `clouds:
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
models:
- name: test-1
  uuid: 00000002-0000-0000-0000-000000000001
  owner: alice@canonical.com
  cloud: test
  region: test-region
  cloud-credential: test-cred
  controller: test
  users:
  - user: alice@canonical.com
    access: admin
  - user: bob@canonical.com
    access: write
- name: test-2
  uuid: 00000002-0000-0000-0000-000000000002
  owner: bob@canonical.com
  cloud: test
  region: test-region
  cloud-credential: test-cred
  controller: test
  users:
  - user: bob@canonical.com
    access: admin
- name: test-3
  uuid: 00000002-0000-0000-0000-000000000003
  owner: bob@canonical.com
  cloud: test
  region: test-region
  cloud-credential: test-cred
  controller: test
  users:
  - user: bob@canonical.com
    access: admin
`

func TestGetModelsByUUIDlUnconfiguredDatabase(t *testing.T) {
	c := qt.New(t)

	var d db.Database
	_, err := d.GetModelsByUUID(context.Background(), nil)
	c.Check(err, qt.ErrorMatches, `database not configured`)
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeServerConfiguration)
}

func (s *dbSuite) TestGetModelsByUUID(c *qt.C) {
	ctx := context.Background()
	err := s.Database.Migrate(context.Background(), true)
	c.Assert(err, qt.Equals, nil)

	env := jimmtest.ParseEnvironment(c, testGetModelsByUUIDEnv)
	env.PopulateDB(c, *s.Database)

	modelUUIDs := []string{
		"00000002-0000-0000-0000-000000000001",
		"00000002-0000-0000-0000-000000000002",
		"00000002-0000-0000-0000-000000000003",
	}
	models, err := s.Database.GetModelsByUUID(ctx, modelUUIDs)
	c.Assert(err, qt.IsNil)
	sort.Slice(models, func(i, j int) bool {
		return models[i].UUID.String < models[j].UUID.String
	})
	c.Check(models[0].UUID.String, qt.Equals, "00000002-0000-0000-0000-000000000001")
	c.Check(models[0].Controller.Name, qt.Not(qt.Equals), "")
	c.Check(models[1].UUID.String, qt.Equals, "00000002-0000-0000-0000-000000000002")
	c.Check(models[1].Controller.Name, qt.Not(qt.Equals), "")
	c.Check(models[2].UUID.String, qt.Equals, "00000002-0000-0000-0000-000000000003")
	c.Check(models[2].Controller.Name, qt.Not(qt.Equals), "")
}

func (s *dbSuite) TestGetModelsByController(c *qt.C) {
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
	u, err := dbmodel.NewIdentity("bob@canonical.com")
	c.Assert(err, qt.IsNil)
	c.Assert(s.Database.DB.Create(&u).Error, qt.IsNil)

	cred := dbmodel.CloudCredential{
		Name:     "test-cred",
		Cloud:    cloud,
		Owner:    *u,
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
		Owner:           *u,
		Controller:      controller,
		CloudRegion:     cloud.Regions[0],
		CloudCredential: cred,
		Type:            "iaas",
		IsController:    true,
		DefaultSeries:   "focal",
		Life:            state.Alive.String(),
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
	}, {
		Name: "test-model-2",
		UUID: sql.NullString{
			String: "00000001-0000-0000-0000-0000-000000000002",
			Valid:  true,
		},
		Owner:           *u,
		Controller:      controller,
		CloudRegion:     cloud.Regions[0],
		CloudCredential: cred,
		Type:            "iaas",
		IsController:    false,
		DefaultSeries:   "focal",
		Life:            state.Alive.String(),
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
	}}
	for _, m := range models {
		c.Assert(s.Database.DB.Create(&m).Error, qt.IsNil)
	}
	foundModels, err := s.Database.GetModelsByController(context.Background(), controller)
	foundModelNames := []string{}
	for _, m := range foundModels {
		foundModelNames = append(foundModelNames, m.Name)
	}
	modelNames := []string{}
	for _, m := range models {
		modelNames = append(modelNames, m.Name)
	}
	c.Assert(err, qt.IsNil)
	c.Assert(foundModelNames, qt.DeepEquals, modelNames)
}

const testCountModelsByControllerEnv = `clouds:
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
models:
- name: test-1
  uuid: 00000002-0000-0000-0000-000000000001
  owner: alice@canonical.com
  cloud: test
  region: test-region
  cloud-credential: test-cred
  controller: test
- name: test-2
  uuid: 00000002-0000-0000-0000-000000000002
  owner: bob@canonical.com
  cloud: test
  region: test-region
  cloud-credential: test-cred
  controller: test
- name: test-3
  uuid: 00000002-0000-0000-0000-000000000003
  owner: bob@canonical.com
  cloud: test
  region: test-region
  cloud-credential: test-cred
  controller: test
`

func (s *dbSuite) TestCountModelsByController(c *qt.C) {
	err := s.Database.Migrate(context.Background(), true)
	c.Assert(err, qt.Equals, nil)

	env := jimmtest.ParseEnvironment(c, testCountModelsByControllerEnv)
	env.PopulateDB(c, *s.Database)
	c.Assert(len(env.Controllers), qt.Equals, 1)
	count, err := s.Database.CountModelsByController(context.Background(), env.Controllers[0].DBObject(c, *s.Database))
	c.Assert(err, qt.IsNil)
	c.Assert(count, qt.Equals, 3)
}
