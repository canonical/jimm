// Copyright 2021 Canonical Ltd.

package db

import (
	"context"

	"gorm.io/gorm/clause"

	"github.com/canonical/jimm/internal/dbmodel"
	"github.com/canonical/jimm/internal/errors"
	"github.com/canonical/jimm/internal/servermon"
)

// SetIdentityModelDefaults sets default model setting values for the controller.
func (d *Database) SetIdentityModelDefaults(ctx context.Context, defaults *dbmodel.IdentityModelDefaults) (err error) {
	const op = errors.Op("db.SetIdentityModelDefaults")

	if err := d.ready(); err != nil {
		return errors.E(op, err)
	}

	durationObserver := servermon.DurationObserver(servermon.DBQueryDurationHistogram, string(op))
	defer durationObserver()
	defer servermon.ErrorCounter(servermon.DBQueryErrorCount, &err, string(op))

	err = d.Transaction(func(d *Database) error {
		db := d.DB.WithContext(ctx)

		dbDefaults := dbmodel.IdentityModelDefaults{
			IdentityName: defaults.IdentityName,
		}
		// try to fetch cloud defaults from the db
		err := d.IdentityModelDefaults(ctx, &dbDefaults)
		if err != nil {
			if errors.ErrorCode(err) == errors.CodeNotFound {
				// if defaults do not exist, we create them
				if err := db.Create(&defaults).Error; err != nil {
					return errors.E(op, dbError(err))
				}
				return nil
			}
			return errors.E(op, err)
		}

		// update defaults
		for k, v := range defaults.Defaults {
			dbDefaults.Defaults[k] = v
		}
		if err := db.Clauses(clause.OnConflict{
			Columns: []clause.Column{
				{Name: "identity_name"},
			},
			DoUpdates: clause.AssignmentColumns([]string{"defaults"}),
		}).Create(&dbDefaults).Error; err != nil {
			return errors.E(op, dbError(err))
		}
		return nil
	})
	if err != nil {
		return errors.E(op, err)
	}
	return nil
}

// IdentityModelDefaults fetches identities defaults.
func (d *Database) IdentityModelDefaults(ctx context.Context, defaults *dbmodel.IdentityModelDefaults) (err error) {
	const op = errors.Op("db.IdentityModelDefaults")

	if err := d.ready(); err != nil {
		return errors.E(op, err)
	}

	durationObserver := servermon.DurationObserver(servermon.DBQueryDurationHistogram, string(op))
	defer durationObserver()
	defer servermon.ErrorCounter(servermon.DBQueryErrorCount, &err, string(op))

	db := d.DB.WithContext(ctx)

	db = db.Where("identity_name = ?", defaults.IdentityName)

	result := db.Preload("Identity").First(&defaults)
	if result.Error != nil {
		err := dbError(result.Error)
		if errors.ErrorCode(err) == errors.CodeNotFound {
			return errors.E(op, errors.CodeNotFound, "identitymodeldefaults not found", err)
		}
		return errors.E(op, err)
	}
	return nil
}
