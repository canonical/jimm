// Copyright 2025 Canonical.

package db_test

import (
	"context"
	"database/sql"

	qt "github.com/frankban/quicktest"

	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
)

func (s *dbSuite) TestAddModelMigration(c *qt.C) {
	err := s.Database.Migrate(context.Background())
	c.Assert(err, qt.Equals, nil)

	cloud := dbmodel.Cloud{
		Name: "test-cloud",
	}
	err = s.Database.AddCloud(context.Background(), &cloud)
	c.Assert(err, qt.IsNil)

	controller := dbmodel.Controller{
		Name:      "test-controller",
		UUID:      "00000000-0000-0000-0000-000000000001",
		CloudName: cloud.Name,
	}
	c.Assert(s.Database.DB.Create(&controller).Error, qt.IsNil)

	migration := dbmodel.IncomingModelMigration{
		ModelUUID:          sql.NullString{String: "00000001-0000-0000-0000-000000000001", Valid: true},
		TargetControllerID: controller.ID,
		UserMapping:        dbmodel.JSON([]byte(`{"local": "external"}`)),
	}
	err = s.Database.AddModelMigration(context.Background(), &migration)
	c.Assert(err, qt.Equals, nil)

	var dbMigration dbmodel.IncomingModelMigration
	result := s.Database.DB.Where("model_uuid = ?", migration.ModelUUID).First(&dbMigration)
	c.Assert(result.Error, qt.Equals, nil)
	c.Assert(dbMigration.ModelUUID, qt.DeepEquals, migration.ModelUUID)
	c.Assert(dbMigration.TargetControllerID, qt.Equals, controller.ID)
	c.Assert(dbMigration.UserMapping, qt.DeepEquals, migration.UserMapping)

	migration.ID = 0 // Reset ID to ensure it is not reused
	err = s.Database.AddModelMigration(context.Background(), &migration)
	c.Assert(err, qt.Not(qt.IsNil))
}

func (s *dbSuite) TestGetModelMigration(c *qt.C) {
	err := s.Database.Migrate(context.Background())
	c.Assert(err, qt.Equals, nil)

	cloud := dbmodel.Cloud{
		Name: "test-cloud",
	}
	err = s.Database.AddCloud(context.Background(), &cloud)
	c.Assert(err, qt.IsNil)

	controller := dbmodel.Controller{
		Name:      "test-controller",
		UUID:      "00000000-0000-0000-0000-000000000001",
		CloudName: cloud.Name,
	}
	c.Assert(s.Database.DB.Create(&controller).Error, qt.IsNil)

	migration := dbmodel.IncomingModelMigration{
		ModelUUID:          sql.NullString{String: "00000001-0000-0000-0000-000000000001", Valid: true},
		TargetControllerID: controller.ID,
		UserMapping:        dbmodel.JSON([]byte(`{"local":"external"}`)),
	}
	c.Assert(s.Database.DB.Create(&migration).Error, qt.IsNil)

	lookup := dbmodel.IncomingModelMigration{ModelUUID: migration.ModelUUID}
	err = s.Database.GetModelMigration(context.Background(), &lookup)
	c.Assert(err, qt.Equals, nil)

	// Not found
	lookup = dbmodel.IncomingModelMigration{ModelUUID: sql.NullString{String: "no-such-uuid", Valid: true}}
	err = s.Database.GetModelMigration(context.Background(), &lookup)
	c.Assert(err, qt.Not(qt.IsNil))
	eError, ok := err.(*errors.Error)
	c.Assert(ok, qt.IsTrue)
	c.Assert(eError.Code, qt.Equals, errors.CodeNotFound)
}

func (s *dbSuite) TestDeleteModelMigration(c *qt.C) {
	err := s.Database.Migrate(context.Background())
	c.Assert(err, qt.Equals, nil)

	cloud := dbmodel.Cloud{
		Name: "test-cloud",
	}
	err = s.Database.AddCloud(context.Background(), &cloud)
	c.Assert(err, qt.IsNil)

	controller := dbmodel.Controller{
		Name:      "test-controller",
		UUID:      "00000000-0000-0000-0000-000000000001",
		CloudName: cloud.Name,
	}
	c.Assert(s.Database.DB.Create(&controller).Error, qt.IsNil)

	migration := dbmodel.IncomingModelMigration{
		ModelUUID:          sql.NullString{String: "00000001-0000-0000-0000-000000000001", Valid: true},
		TargetControllerID: controller.ID,
		UserMapping:        dbmodel.JSON([]byte(`{"local":"external"}`)),
	}
	c.Assert(s.Database.DB.Create(&migration).Error, qt.IsNil)

	err = s.Database.DeleteModelMigration(context.Background(), &migration)
	c.Assert(err, qt.Equals, nil)

	var dbMigration dbmodel.IncomingModelMigration
	result := s.Database.DB.Where("model_uuid = ?", migration.ModelUUID).First(&dbMigration)
	c.Assert(result.Error, qt.Not(qt.IsNil))
}
