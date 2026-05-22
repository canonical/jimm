// Copyright 2026 Canonical.

package db

import (
	"context"

	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/servermon"
)

// AddControllerBootstrap stores a pending controller bootstrap reservation.
func (d *Database) AddControllerBootstrap(ctx context.Context, bootstrap *dbmodel.ControllerBootstrap) (err error) {
	const op = "db.AddControllerBootstrap"
	if err := d.ready(); err != nil {
		return err
	}

	durationObserver := servermon.DurationObserver(servermon.DBQueryDurationHistogram, op)
	defer durationObserver()
	defer servermon.ErrorCounter(servermon.DBQueryErrorCount, &err, op)

	if err := d.DB.WithContext(ctx).Create(bootstrap).Error; err != nil {
		return dbError(err)
	}
	return nil
}

// GetControllerBootstrap returns a pending controller bootstrap by id, job id, or name.
func (d *Database) GetControllerBootstrap(ctx context.Context, bootstrap *dbmodel.ControllerBootstrap) (err error) {
	const op = "db.GetControllerBootstrap"
	if err := d.ready(); err != nil {
		return err
	}

	durationObserver := servermon.DurationObserver(servermon.DBQueryDurationHistogram, op)
	defer durationObserver()
	defer servermon.ErrorCounter(servermon.DBQueryErrorCount, &err, op)

	db := d.DB.WithContext(ctx)
	switch {
	case bootstrap.ID != 0:
		db = db.Where("id = ?", bootstrap.ID)
	case bootstrap.JobID.Valid:
		db = db.Where("job_id = ?", bootstrap.JobID.Int64)
	case bootstrap.Name != "":
		db = db.Where("name = ?", bootstrap.Name)
	default:
		return errors.Codef(errors.CodeBadRequest, "controller bootstrap id, job id, or name must be provided")
	}
	if err := db.First(bootstrap).Error; err != nil {
		err = dbError(err)
		if errors.ErrorCode(err) == errors.CodeNotFound {
			return errors.Codef(errors.CodeNotFound, "controller bootstrap not found")
		}
		return err
	}
	return nil
}

// UpdateControllerBootstrap updates an existing pending controller bootstrap reservation.
func (d *Database) UpdateControllerBootstrap(ctx context.Context, bootstrap *dbmodel.ControllerBootstrap) (err error) {
	const op = "db.UpdateControllerBootstrap"
	if bootstrap.ID == 0 {
		return errors.Codef(errors.CodeNotFound, "controller bootstrap not found")
	}
	if err := d.ready(); err != nil {
		return err
	}

	durationObserver := servermon.DurationObserver(servermon.DBQueryDurationHistogram, op)
	defer durationObserver()
	defer servermon.ErrorCounter(servermon.DBQueryErrorCount, &err, op)

	if err := d.DB.WithContext(ctx).Save(bootstrap).Error; err != nil {
		return dbError(err)
	}
	return nil
}

// DeleteControllerBootstrap removes a pending controller bootstrap reservation.
func (d *Database) DeleteControllerBootstrap(ctx context.Context, bootstrap *dbmodel.ControllerBootstrap) (err error) {
	const op = "db.DeleteControllerBootstrap"
	if bootstrap.ID == 0 {
		return errors.Codef(errors.CodeNotFound, "controller bootstrap not found")
	}
	if err := d.ready(); err != nil {
		return err
	}

	durationObserver := servermon.DurationObserver(servermon.DBQueryDurationHistogram, op)
	defer durationObserver()
	defer servermon.ErrorCounter(servermon.DBQueryErrorCount, &err, op)

	if err := d.DB.WithContext(ctx).Delete(bootstrap).Error; err != nil {
		return dbError(err)
	}
	return nil
}

// ListControllerBootstraps returns all pending controller bootstrap reservations ordered by name.
func (d *Database) ListControllerBootstraps(ctx context.Context) (bootstraps []dbmodel.ControllerBootstrap, err error) {
	const op = "db.ListControllerBootstraps"
	if err := d.ready(); err != nil {
		return nil, err
	}

	durationObserver := servermon.DurationObserver(servermon.DBQueryDurationHistogram, op)
	defer durationObserver()
	defer servermon.ErrorCounter(servermon.DBQueryErrorCount, &err, op)

	if err := d.DB.WithContext(ctx).Order("name asc").Find(&bootstraps).Error; err != nil {
		return nil, dbError(err)
	}
	return bootstraps, nil
}
