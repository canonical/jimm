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
