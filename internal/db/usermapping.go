// Copyright 2025 Canonical.

package db

import (
	"context"

	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/servermon"
)

// AddUserMapping stores information mapping a local user to an external user.
func (d *Database) AddUserMapping(ctx context.Context, userMapping *dbmodel.UserMapping) (err error) {
	const op = "db.AddUserMapping"
	if err := d.ready(); err != nil {
		return err
	}

	durationObserver := servermon.DurationObserver(servermon.DBQueryDurationHistogram, op)
	defer durationObserver()
	defer servermon.ErrorCounter(servermon.DBQueryErrorCount, &err, op)

	db := d.DB.WithContext(ctx)

	if err := db.Create(userMapping).Error; err != nil {
		return dbError(err)
	}
	return nil
}

// GetUserMapping returns user mapping info based on the model UUID and local user.
func (d *Database) GetUserMapping(ctx context.Context, userMapping *dbmodel.UserMapping) (err error) {
	const op = "db.GetUserMapping"
	if err := d.ready(); err != nil {
		return err
	}

	durationObserver := servermon.DurationObserver(servermon.DBQueryDurationHistogram, op)
	defer durationObserver()
	defer servermon.ErrorCounter(servermon.DBQueryErrorCount, &err, op)

	db := d.DB.WithContext(ctx)
	switch {
	case userMapping.ModelUUID.Valid && userMapping.LocalUser != "":
		db = db.Where("model_uuid = ? AND local_user = ?", userMapping.ModelUUID.String, userMapping.LocalUser)
	case !userMapping.ModelUUID.Valid:
		return errors.Codef(errors.CodeBadRequest, "missing model UUID")
	case userMapping.LocalUser == "":
		return errors.Codef(errors.CodeBadRequest, "missing local user")
	default:
		return errors.Codef(errors.CodeBadRequest, "invalid parameters")
	}

	if err := db.First(&userMapping).Error; err != nil {
		err = dbError(err)
		if errors.ErrorCode(err) == errors.CodeNotFound {
			return errors.Codef(errors.CodeNotFound, "user mapping not found")
		}
		return err
	}
	return nil
}

// DeleteUserMapping removes a user mapping from the database.
func (d *Database) DeleteUserMapping(ctx context.Context, userMapping *dbmodel.UserMapping) (err error) {
	const op = "db.DeleteUserMapping"
	if err := d.ready(); err != nil {
		return err
	}

	durationObserver := servermon.DurationObserver(servermon.DBQueryDurationHistogram, op)
	defer durationObserver()
	defer servermon.ErrorCounter(servermon.DBQueryErrorCount, &err, op)

	db := d.DB.WithContext(ctx)
	if err := db.Delete(userMapping).Error; err != nil {
		return dbError(err)
	}
	return nil
}

// DeleteUserMappingsByModelUUID removes all user mappings for a given model UUID.
func (d *Database) DeleteUserMappingsByModelUUID(ctx context.Context, modelUUID string) (err error) {
	const op = "db.DeleteUserMappingsByModelUUID"
	if err := d.ready(); err != nil {
		return err
	}

	durationObserver := servermon.DurationObserver(servermon.DBQueryDurationHistogram, op)
	defer durationObserver()
	defer servermon.ErrorCounter(servermon.DBQueryErrorCount, &err, op)

	db := d.DB.WithContext(ctx)
	if err := db.Where("model_uuid = ?", modelUUID).Delete(&dbmodel.UserMapping{}).Error; err != nil {
		return dbError(err)
	}
	return nil
}
