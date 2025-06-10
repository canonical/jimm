// Copyright 2025 Canonical.

package db

import (
	"context"

	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/servermon"
)

// AddModelMigration stores information about an incoming model migration.
//   - returns an error with code errors.CodeAlreadyExists if
//     a migration row with the same model UUID already exists.
func (d *Database) AddModelMigration(ctx context.Context, modelMigration *dbmodel.IncomingModelMigration) (err error) {
	const op = errors.Op("db.AddModelMigration")
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

// GetModelMigration returns model migration information based on the model UUID.
func (d *Database) GetModelMigration(ctx context.Context, modelMigration *dbmodel.IncomingModelMigration) (err error) {
	const op = errors.Op("db.GetModelMigration")
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

	if err := db.First(&modelMigration).Error; err != nil {
		err = dbError(err)
		if errors.ErrorCode(err) == errors.CodeNotFound {
			return errors.E(op, err, "model migration not found")
		}
		return errors.E(op, dbError(err))
	}
	return nil
}

// DeleteModelMigration removes a model migration entry from the database.
func (d *Database) DeleteModelMigration(ctx context.Context, modelMigration *dbmodel.IncomingModelMigration) (err error) {
	const op = errors.Op("db.DeleteModelMigration")
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
