// Copyright 2021 Canonical Ltd.

package db

import (
	"context"

	"gorm.io/gorm/clause"

	"github.com/CanonicalLtd/jimm/internal/dbmodel"
	"github.com/CanonicalLtd/jimm/internal/errors"
)

// SetUserModelDefaults sets default model setting values for the controller.
func (d *Database) SetUserModelDefaults(ctx context.Context, defaults *dbmodel.UserModelDefaults) error {
	const op = errors.Op("db.SetUserModelDefaults")

	err := d.Transaction(func(d *Database) error {
		db := d.DB.WithContext(ctx)

		dbDefaults := dbmodel.UserModelDefaults{
			Username: defaults.Username,
		}
		// try to fetch cloud defaults from the db
		err := d.UserModelDefaults(ctx, &dbDefaults)
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
				{Name: "username"},
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

// UserModelDefaults fetches user defaults.
func (d *Database) UserModelDefaults(ctx context.Context, defaults *dbmodel.UserModelDefaults) error {
	const op = errors.Op("db.UserModelDefaults")

	if err := d.ready(); err != nil {
		return errors.E(op, err)
	}
	db := d.DB.WithContext(ctx)

	db = db.Where("username = ?", defaults.Username)

	result := db.Preload("User").First(&defaults)
	if result.Error != nil {
		err := dbError(result.Error)
		if errors.ErrorCode(err) == errors.CodeNotFound {
			return errors.E(op, errors.CodeNotFound, "usermodeldefaults not found", err)
		}
		return errors.E(op, err)
	}
	return nil
}
