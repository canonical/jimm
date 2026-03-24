// Copyright 2025 Canonical.

package db_test

import (
	"context"
	"database/sql"
	"time"

	qt "github.com/frankban/quicktest"

	"github.com/canonical/jimm/v3/internal/db"
	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
)

func (s *dbSuite) setupModelMigrationTest(c *qt.C) dbmodel.Controller {
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
	return controller
}

func (s *dbSuite) TestAddModelMigration(c *qt.C) {
	controller := s.setupModelMigrationTest(c)

	migration := dbmodel.IncomingModelMigration{
		ModelUUID:          sql.NullString{String: "00000001-0000-0000-0000-000000000001", Valid: true},
		TargetControllerID: controller.ID,
		UserMapping:        dbmodel.StringMap{"local": "external"},
	}
	err := s.Database.AddOrUpdateIncomingModelMigration(context.Background(), &migration)
	c.Assert(err, qt.Equals, nil)

	var dbMigration dbmodel.IncomingModelMigration
	result := s.Database.DB.Where("model_uuid = ?", migration.ModelUUID).First(&dbMigration)
	c.Assert(result.Error, qt.Equals, nil)
	c.Assert(dbMigration.ModelUUID, qt.DeepEquals, migration.ModelUUID)
	c.Assert(dbMigration.TargetControllerID, qt.Equals, controller.ID)
	c.Assert(dbMigration.UserMapping, qt.DeepEquals, migration.UserMapping)
	c.Assert(dbMigration.UpdatedAt, qt.Not(qt.Equals), time.Time{})
}

func (s *dbSuite) TestReplaceModelMigration(c *qt.C) {
	controller := s.setupModelMigrationTest(c)

	migration := dbmodel.IncomingModelMigration{
		ModelUUID:          sql.NullString{String: "00000001-0000-0000-0000-000000000001", Valid: true},
		TargetControllerID: controller.ID,
		UserMapping:        dbmodel.StringMap{"local": "external"},
	}
	err := s.Database.AddOrUpdateIncomingModelMigration(context.Background(), &migration)
	c.Assert(err, qt.Equals, nil)

	newUserMapping := dbmodel.StringMap{"local": "new-external"}
	migration.UserMapping = newUserMapping
	err = s.Database.AddOrUpdateIncomingModelMigration(context.Background(), &migration)
	c.Assert(err, qt.Equals, nil)

	// Verify that the migration was updated
	var dbMigration dbmodel.IncomingModelMigration
	result := s.Database.DB.Where("model_uuid = ?", migration.ModelUUID).First(&dbMigration)
	c.Assert(result.Error, qt.Equals, nil)
	c.Assert(dbMigration.ModelUUID, qt.DeepEquals, migration.ModelUUID)
	c.Assert(dbMigration.TargetControllerID, qt.Equals, controller.ID)
	c.Assert(dbMigration.UserMapping, qt.DeepEquals, newUserMapping)
}

func (s *dbSuite) TestGetModelMigration(c *qt.C) {
	controller := s.setupModelMigrationTest(c)

	migration := dbmodel.IncomingModelMigration{
		ModelUUID:          sql.NullString{String: "00000001-0000-0000-0000-000000000001", Valid: true},
		TargetControllerID: controller.ID,
		UserMapping:        dbmodel.StringMap{"local": "external"},
	}
	c.Assert(s.Database.DB.Create(&migration).Error, qt.IsNil)

	lookup := dbmodel.IncomingModelMigration{ModelUUID: migration.ModelUUID}
	err := s.Database.GetIncomingModelMigration(context.Background(), &lookup)
	c.Assert(err, qt.Equals, nil)
	c.Assert(lookup.ModelUUID, qt.DeepEquals, migration.ModelUUID)
	c.Assert(lookup.TargetControllerID, qt.Equals, controller.ID)
	c.Assert(lookup.UserMapping, qt.DeepEquals, migration.UserMapping)

	// Not found
	lookup = dbmodel.IncomingModelMigration{ModelUUID: sql.NullString{String: "no-such-uuid", Valid: true}}
	err = s.Database.GetIncomingModelMigration(context.Background(), &lookup)
	c.Assert(err, qt.Not(qt.IsNil))
	c.Assert(errors.ErrorCode(err), qt.Equals, errors.CodeNotFound)
}

func (s *dbSuite) TestGetModelMigrationWithLock_NoWait(c *qt.C) {
	controller := s.setupModelMigrationTest(c)

	migration := dbmodel.IncomingModelMigration{
		ModelUUID:          sql.NullString{String: "00000001-0000-0000-0000-000000000001", Valid: true},
		TargetControllerID: controller.ID,
		UserMapping:        dbmodel.StringMap{"local": "external"},
	}
	c.Assert(s.Database.DB.Create(&migration).Error, qt.IsNil)

	notifyChan := make(chan struct{})
	noWait := true
	go func() {
		// Lock the migration
		err := s.Database.Transaction(func(d *db.Database) error {
			lookup := dbmodel.IncomingModelMigration{
				ModelUUID: migration.ModelUUID,
			}
			err := d.GetIncomingModelMigrationWithLock(context.Background(), &lookup, noWait)
			c.Check(err, qt.Equals, nil)
			c.Check(lookup.ModelUUID, qt.DeepEquals, migration.ModelUUID)
			close(notifyChan)
			<-c.Context().Done()
			return nil
		})
		c.Check(err, qt.IsNil)
	}()

	lookup := dbmodel.IncomingModelMigration{
		ModelUUID: migration.ModelUUID,
	}
	<-notifyChan // Wait for the migration to be locked
	err := s.Database.GetIncomingModelMigrationWithLock(context.Background(), &lookup, noWait)
	c.Check(err, qt.IsNotNil)
	c.Check(err, qt.ErrorMatches, ".*could not obtain lock on row.*")
}

func (s *dbSuite) TestGetModelMigrationWithLock_Wait(c *qt.C) {
	controller := s.setupModelMigrationTest(c)

	migration := dbmodel.IncomingModelMigration{
		ModelUUID:          sql.NullString{String: "00000001-0000-0000-0000-000000000001", Valid: true},
		TargetControllerID: controller.ID,
		UserMapping:        dbmodel.StringMap{"local": "external"},
	}
	c.Assert(s.Database.DB.Create(&migration).Error, qt.IsNil)

	notifyChan := make(chan struct{})
	done := make(chan struct{})
	noWait := false
	go func() {
		// Lock the migration
		err := s.Database.Transaction(func(d *db.Database) error {
			lookup := dbmodel.IncomingModelMigration{
				ModelUUID: migration.ModelUUID,
			}
			err := d.GetIncomingModelMigrationWithLock(context.Background(), &lookup, noWait)
			c.Check(err, qt.Equals, nil)
			c.Check(lookup.ModelUUID, qt.DeepEquals, migration.ModelUUID)
			close(notifyChan)
			// Sleep to simulate holding the lock, since we can't tell
			// when the main thread is blocked on the lock.
			<-time.After(1 * time.Second)
			return nil
		})
		c.Check(err, qt.IsNil)
		close(done)
	}()

	lookup := dbmodel.IncomingModelMigration{
		ModelUUID: migration.ModelUUID,
	}
	<-notifyChan // Wait for the migration to be locked

	// Check that we cannot obtain the lock immediately
	err := s.Database.GetIncomingModelMigrationWithLock(context.Background(), &lookup, true)
	c.Check(err, qt.IsNotNil)
	c.Check(err, qt.ErrorMatches, ".*could not obtain lock on row.*")

	// Now we should be able to obtain the lock if we wait
	err = s.Database.GetIncomingModelMigrationWithLock(context.Background(), &lookup, noWait)
	c.Check(err, qt.Equals, nil)
	c.Check(lookup.ModelUUID, qt.DeepEquals, migration.ModelUUID)
	<-done // Wait for the goroutine to finish
}

func (s *dbSuite) TestGetModelMigrationsCreatedBefore(c *qt.C) {
	controller := s.setupModelMigrationTest(c)

	migration := dbmodel.IncomingModelMigration{
		ModelUUID:          sql.NullString{String: "00000001-0000-0000-0000-000000000001", Valid: true},
		TargetControllerID: controller.ID,
		UserMapping:        dbmodel.StringMap{"local": "external"},
	}
	c.Assert(s.Database.DB.Create(&migration).Error, qt.IsNil)

	migrations, err := s.Database.GetIncomingModelMigrationsCreatedBefore(context.Background(), migration.CreatedAt.Add(1*time.Hour))
	c.Assert(err, qt.Equals, nil)
	c.Assert(migrations, qt.HasLen, 1)
	c.Assert(migrations[0].ModelUUID, qt.DeepEquals, migration.ModelUUID)

	migrations, err = s.Database.GetIncomingModelMigrationsCreatedBefore(context.Background(), migration.CreatedAt.Add(-1*time.Hour))
	c.Assert(err, qt.Equals, nil)
	c.Assert(migrations, qt.HasLen, 0)
}

func (s *dbSuite) TestDeleteModelMigration(c *qt.C) {
	controller := s.setupModelMigrationTest(c)

	migration := dbmodel.IncomingModelMigration{
		ModelUUID:          sql.NullString{String: "00000001-0000-0000-0000-000000000001", Valid: true},
		TargetControllerID: controller.ID,
		UserMapping:        dbmodel.StringMap{"local": "external"},
	}
	c.Assert(s.Database.DB.Create(&migration).Error, qt.IsNil)

	err := s.Database.DeleteIncomingModelMigration(context.Background(), &migration)
	c.Assert(err, qt.Equals, nil)

	var dbMigration dbmodel.IncomingModelMigration
	result := s.Database.DB.Where("model_uuid = ?", migration.ModelUUID).First(&dbMigration)
	c.Assert(result.Error, qt.Not(qt.IsNil))
}
