// Copyright 2021 Canonical Ltd.

package db

import (
	"context"

	"github.com/juju/names/v4"
	"gorm.io/gorm/clause"

	"github.com/CanonicalLtd/jimm/internal/dbmodel"
	"github.com/CanonicalLtd/jimm/internal/errors"
)

// SetCloudDefaults sets default model setting values for the specified cloud/region.
func (d *Database) SetCloudDefaults(ctx context.Context, defaults *dbmodel.CloudDefaults) error {
	const op = errors.Op("db.SetCloudDefaults")

	err := d.Transaction(func(d *Database) error {
		db := d.DB.WithContext(ctx)

		dbDefaults := dbmodel.CloudDefaults{
			UserID:  defaults.UserID,
			CloudID: defaults.CloudID,
			Cloud: dbmodel.Cloud{
				Name: defaults.Cloud.Name,
			},
			Region: defaults.Region,
		}
		// try to fetch cloud defaults from the db
		err := d.CloudDefaults(ctx, &dbDefaults)
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
				{Name: "user_id"},
				{Name: "cloud_id"},
				{Name: "region"},
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

// UnsetCloudDefaults unsets default model setting values for the specified cloud/region.
func (d *Database) UnsetCloudDefaults(ctx context.Context, defaults *dbmodel.CloudDefaults, keys []string) error {
	const op = errors.Op("db.UpsertCloudDefaults")

	err := d.Transaction(func(d *Database) error {
		db := d.DB.WithContext(ctx)

		dbDefaults := dbmodel.CloudDefaults{
			UserID:  defaults.UserID,
			CloudID: defaults.CloudID,
			Cloud: dbmodel.Cloud{
				Name: defaults.Cloud.Name,
			},
			Region: defaults.Region,
		}
		// try to fetch cloud defaults from the db
		err := d.CloudDefaults(ctx, &dbDefaults)
		if err != nil {
			return errors.E(op, err)
		}

		// update defaults
		for _, key := range keys {
			delete(dbDefaults.Defaults, key)
		}
		if err := db.Clauses(clause.OnConflict{
			Columns: []clause.Column{
				{Name: "user_id"},
				{Name: "cloud_id"},
				{Name: "region"},
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

// CloudDefaults fetches cloud defaults based on user, cloud name or id and region name.
func (d *Database) CloudDefaults(ctx context.Context, defaults *dbmodel.CloudDefaults) error {
	const op = errors.Op("db.CloudDefaults")

	if err := d.ready(); err != nil {
		return errors.E(op, err)
	}
	db := d.DB.WithContext(ctx)

	db = db.Where("user_id = ?", defaults.UserID)
	db = db.Joins("JOIN clouds ON clouds.id = cloud_defaults.cloud_id")
	if defaults.CloudID != 0 {
		db = db.Where("clouds.id = ?", defaults.CloudID)
	} else {
		db = db.Where("clouds.name = ?", defaults.Cloud.Name)
	}
	db = db.Where("region = ?", defaults.Region)

	result := db.Preload("User").Preload("Cloud").First(&defaults)
	if result.Error != nil {
		err := dbError(result.Error)
		if errors.ErrorCode(err) == errors.CodeNotFound {
			return errors.E(op, errors.CodeNotFound, "cloudregiondefaults not found", err)
		}
		return errors.E(op, err)
	}
	return nil
}

// ModelDefaultsForCloud returns the default config values for the specified cloud.
func (d *Database) ModelDefaultsForCloud(ctx context.Context, user *dbmodel.User, cloud names.CloudTag) ([]dbmodel.CloudDefaults, error) {
	const op = errors.Op("db.ModelDefaultsForCloud")

	if err := d.ready(); err != nil {
		return nil, errors.E(op, err)
	}
	db := d.DB.WithContext(ctx)

	db = db.Where("user_id = ?", user.Username)
	db = db.Joins("JOIN clouds ON clouds.id = cloud_defaults.cloud_id")
	db = db.Where("clouds.name = ?", cloud.Id())

	var defaults []dbmodel.CloudDefaults
	result := db.Preload("User").Preload("Cloud").Find(&defaults)
	if result.Error != nil {
		return nil, errors.E(op, dbError(result.Error))
	}
	return defaults, nil
}
