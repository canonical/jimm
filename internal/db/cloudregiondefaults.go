// Copyright 2021 Canonical Ltd.

package db

import (
	"context"

	"gorm.io/gorm/clause"

	"github.com/CanonicalLtd/jimm/internal/dbmodel"
	"github.com/CanonicalLtd/jimm/internal/errors"
)

// UpsertCloudRegionDefaults sets or updates default model setting values for the specified cloud/region.
func (d *Database) UpsertCloudRegionDefaults(ctx context.Context, defaults *dbmodel.CloudRegionDefaults) error {
	const op = errors.Op("db.UpsertCloudRegionDefaults")

	if err := d.ready(); err != nil {
		return errors.E(op, err)
	}
	db := d.DB.WithContext(ctx)
	if err := db.Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "user_id"},
			{Name: "cloud_region_id"},
		},
		DoUpdates: clause.AssignmentColumns([]string{"defaults"}),
	}).Create(&defaults).Error; err != nil {
		return errors.E(op, dbError(err))
	}
	return nil
}

// ModelDefaultsForCloud returns the default config values for the specified cloud.
func (d *Database) ModelDefaultsForCloud(ctx context.Context, user *dbmodel.User, cloud *dbmodel.Cloud) ([]dbmodel.CloudRegionDefaults, error) {
	const op = errors.Op("db.ModelDefaultsForCloud")
	if err := d.ready(); err != nil {
		return nil, errors.E(op, err)
	}
	db := d.DB.WithContext(ctx)

	db = db.Where("user_id = ?", user.Username)
	db = db.Joins("JOIN cloud_regions AS cr ON cr.id = cloud_region_defaults.cloud_region_id")
	db = db.Where("cr.cloud_name = ?", cloud.Name)

	var defaults []dbmodel.CloudRegionDefaults
	result := db.Preload("User").Preload("CloudRegion").Find(&defaults)
	if result.Error != nil {
		return nil, errors.E(op, dbError(result.Error))
	}
	return defaults, nil
}
