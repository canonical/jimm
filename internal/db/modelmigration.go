// Copyright 2025 Canonical.

package db

import (
	"context"
	"time"

	"gorm.io/gorm/clause"

	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/servermon"
)

// AddOrUpdateIncomingModelMigration stores information about an incoming model migration
// if it does not already exist, or updates it if it does.
// It must be run within a transaction as it will lock the existing row for updates
// if the model migration already exists.
func (d *Database) AddOrUpdateIncomingModelMigration(ctx context.Context, modelMigration *dbmodel.IncomingModelMigration) (err error) {
	const op = errors.Op("db.AddIncomingModelMigration")
	if err := d.ready(); err != nil {
		return errors.E(op, err)
	}

	durationObserver := servermon.DurationObserver(servermon.DBQueryDurationHistogram, string(op))
	defer durationObserver()
	defer servermon.ErrorCounter(servermon.DBQueryErrorCount, &err, string(op))

	lookup := dbmodel.IncomingModelMigration{
		ModelUUID: modelMigration.ModelUUID,
	}
	// Set noWait to true to ensure that if the row is locked by another transaction,
	// we will return an error immediately instead of waiting.
	err = d.GetIncomingModelMigrationWithLock(ctx, &lookup, true)
	if err == nil {
		// If the model migration already exists, update it.
		modelMigration.ID = lookup.ID
	} else if err != nil && errors.ErrorCode(err) != errors.CodeNotFound {
		return errors.E(op, err)
	}

	db := d.DB.WithContext(ctx)

	if err := db.Save(modelMigration).Error; err != nil {
		return errors.E(op, dbError(err))
	}
	return nil
}

// GetIncomingMigrationWithLock retrieves an incoming model migration locking the row for updates.
// This must be run within a transaction for the lock to be effective.
// if `noWait` is true, the function will return an error if the lock cannot be acquired immediately.
func (d *Database) GetIncomingModelMigrationWithLock(ctx context.Context, modelMigration *dbmodel.IncomingModelMigration, noWait bool) (err error) {
	const op = errors.Op("db.GetIncomingMigrationWithLock")

	durationObserver := servermon.DurationObserver(servermon.DBQueryDurationHistogram, string(op))
	defer durationObserver()
	defer servermon.ErrorCounter(servermon.DBQueryErrorCount, &err, string(op))

	db := d.DB.WithContext(ctx)
	switch {
	case modelMigration.ModelUUID.Valid:
		db = db.Where("model_uuid = ?", modelMigration.ModelUUID.String)
	default:
		return errors.E(op, "missing uuid", errors.CodeBadRequest)
	}

	if noWait {
		db = db.Clauses(clause.Locking{Strength: "UPDATE", Options: "NOWAIT"})
	} else {
		db = db.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	if err := db.First(&modelMigration).Error; err != nil {
		err = dbError(err)
		if errors.ErrorCode(err) == errors.CodeNotFound {
			return errors.E(op, err, "model migration not found")
		}
		return errors.E(op, err)
	}
	return nil
}

// GetIncomingModelMigration returns model migration information based on the model UUID.
func (d *Database) GetIncomingModelMigration(ctx context.Context, modelMigration *dbmodel.IncomingModelMigration) (err error) {
	const op = errors.Op("db.GetIncomingModelMigration")
	if err := d.ready(); err != nil {
		return errors.E(op, err)
	}

	durationObserver := servermon.DurationObserver(servermon.DBQueryDurationHistogram, string(op))
	defer durationObserver()
	defer servermon.ErrorCounter(servermon.DBQueryErrorCount, &err, string(op))

	db := d.DB.WithContext(ctx)
	switch {
	case modelMigration.ModelUUID.Valid:
		db = db.Where("model_uuid = ?", modelMigration.ModelUUID.String)
	default:
		return errors.E(op, "missing uuid", errors.CodeBadRequest)
	}

	if err := db.Preload("TargetController").First(&modelMigration).Error; err != nil {
		err = dbError(err)
		if errors.ErrorCode(err) == errors.CodeNotFound {
			return errors.E(op, err, "model migration not found")
		}
		return errors.E(op, err)
	}
	return nil
}

// DeleteIncomingModelMigration removes a model migration entry from the database.
func (d *Database) DeleteIncomingModelMigration(ctx context.Context, modelMigration *dbmodel.IncomingModelMigration) (err error) {
	const op = errors.Op("db.DeleteIncomingModelMigration")
	if err := d.ready(); err != nil {
		return errors.E(op, err)
	}

	durationObserver := servermon.DurationObserver(servermon.DBQueryDurationHistogram, string(op))
	defer durationObserver()
	defer servermon.ErrorCounter(servermon.DBQueryErrorCount, &err, string(op))

	db := d.DB.WithContext(ctx)
	if err := db.Delete(modelMigration).Error; err != nil {
		return errors.E(op, dbError(err))
	}
	return nil
}

// GetIncomingModelMigrationsCreatedBefore returns all incoming model migrations created before the specified time.
func (d *Database) GetIncomingModelMigrationsCreatedBefore(ctx context.Context, createBefore time.Time) (migrations []dbmodel.IncomingModelMigration, err error) {
	const op = errors.Op("db.GetIncomingModelMigrations")
	if err := d.ready(); err != nil {
		return nil, errors.E(op, err)
	}

	durationObserver := servermon.DurationObserver(servermon.DBQueryDurationHistogram, string(op))
	defer durationObserver()
	defer servermon.ErrorCounter(servermon.DBQueryErrorCount, &err, string(op))

	db := d.DB.WithContext(ctx)

	if err := db.Where("created_at < ?", createBefore).Find(&migrations).Error; err != nil {
		return nil, errors.E(op, dbError(err))
	}
	return migrations, nil
}
