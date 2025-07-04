// Copyright 2025 Canonical.

package db

import (
	"context"
	"time"

	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/servermon"
)

// AddIncomingModelMigration stores information about an incoming model migration.
//   - returns an error with code errors.CodeAlreadyExists if
//     a migration row with the same model UUID already exists.
func (d *Database) AddIncomingModelMigration(ctx context.Context, modelMigration *dbmodel.IncomingModelMigration) (err error) {
	const op = errors.Op("db.AddIncomingModelMigration")
	if err := d.ready(); err != nil {
		return errors.E(op, err)
	}

	durationObserver := servermon.DurationObserver(servermon.DBQueryDurationHistogram, string(op))
	defer durationObserver()
	defer servermon.ErrorCounter(servermon.DBQueryErrorCount, &err, string(op))

	db := d.DB.WithContext(ctx)

	if err := db.Create(modelMigration).Error; err != nil {
		return errors.E(op, dbError(err))
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
		return errors.E(op, dbError(err))
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
