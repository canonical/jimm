// Copyright 2020 Canonical Ltd.

package db_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	qt "github.com/frankban/quicktest"
	"gorm.io/gorm"

	"github.com/CanonicalLtd/jimm/internal/db"
	"github.com/CanonicalLtd/jimm/internal/dbmodel"
	"github.com/CanonicalLtd/jimm/internal/errors"
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
		OwnerID:           u.Username,
		ControllerID:      controller.ID,
		CloudRegionID:     cloud.Regions[0].ID,
		CloudCredentialID: cred.ID,
		Type:              "iaas",
		DefaultSeries:     "warty",
		Life:              "alive",
		Status: dbmodel.Status{
			Status: "available",
			Since: sql.NullTime{
				Time:  time.Now().Truncate(time.Millisecond),
				Valid: true,
			},
		},
		SLA: dbmodel.SLA{
			Level: "unsupported",
		},
		Users: []dbmodel.UserModelAccess{{
			UserID: u.ID,
			Access: "admin",
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
		OwnerID:           u.Username,
		ControllerID:      controller.ID,
		CloudRegionID:     cloud.Regions[0].ID,
		CloudCredentialID: cred.ID,
		Type:              "iaas",
		DefaultSeries:     "warty",
		Life:              "alive",
		Status: dbmodel.Status{
			Status: "available",
			Since: sql.NullTime{
				Time:  time.Now().Truncate(time.Millisecond),
				Valid: true,
			},
		},
		SLA: dbmodel.SLA{
			Level: "unsupported",
		},
		Users: []dbmodel.UserModelAccess{{
			UserID: u.ID,
			Access: "admin",
		}},
	}
	err = s.Database.AddModel(context.Background(), &model)
	c.Assert(err, qt.Equals, nil)

	dbModel := dbmodel.Model{
		UUID: model.UUID,
	}
	err = s.Database.GetModel(context.Background(), &dbModel)
	c.Assert(err, qt.Equals, nil)
	c.Assert(dbModel, qt.DeepEquals, model)

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
		OwnerID:           u.Username,
		ControllerID:      controller.ID,
		CloudRegionID:     cloud.Regions[0].ID,
		CloudCredentialID: cred.ID,
		Type:              "iaas",
		DefaultSeries:     "warty",
		Life:              "alive",
		Status: dbmodel.Status{
			Status: "available",
			Since: sql.NullTime{
				Time:  time.Now().Truncate(time.Millisecond),
				Valid: true,
			},
		},
		SLA: dbmodel.SLA{
			Level: "unsupported",
		},
		Users: []dbmodel.UserModelAccess{{
			UserID: u.ID,
			Access: "admin",
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
		OwnerID:           u.Username,
		ControllerID:      controller.ID,
		CloudRegionID:     cloud.Regions[0].ID,
		CloudCredentialID: cred.ID,
		Type:              "iaas",
		DefaultSeries:     "warty",
		Life:              "alive",
		Status: dbmodel.Status{
			Status: "available",
			Since: sql.NullTime{
				Time:  time.Now().Truncate(time.Millisecond),
				Valid: true,
			},
		},
		SLA: dbmodel.SLA{
			Level: "unsupported",
		},
		Users: []dbmodel.UserModelAccess{{
			UserID: u.ID,
			Access: "admin",
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
		OwnerID:           u.Username,
		ControllerID:      controller.ID,
		CloudRegionID:     cloud.Regions[0].ID,
		CloudCredentialID: cred1.ID,
		Type:              "iaas",
		DefaultSeries:     "warty",
		Life:              "alive",
		Status: dbmodel.Status{
			Status: "available",
			Since: sql.NullTime{
				Time:  time.Now().UTC().Truncate(time.Millisecond),
				Valid: true,
			},
		},
		SLA: dbmodel.SLA{
			Level: "unsupported",
		},
		Users: []dbmodel.UserModelAccess{{
			UserID: u.ID,
			Access: "admin",
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
		OwnerID:           u.Username,
		ControllerID:      controller.ID,
		CloudRegionID:     cloud.Regions[0].ID,
		CloudCredentialID: cred2.ID,
		Type:              "iaas",
		DefaultSeries:     "warty",
		Life:              "alive",
		Status: dbmodel.Status{
			Status: "available",
			Since: sql.NullTime{
				Time:  time.Now().UTC().Truncate(time.Millisecond),
				Valid: true,
			},
		},
		SLA: dbmodel.SLA{
			Level: "unsupported",
		},
		Users: []dbmodel.UserModelAccess{{
			UserID: u.ID,
			Access: "admin",
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
		OwnerID:           u.Username,
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
