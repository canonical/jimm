// Copyright 2021 Canonical Ltd.

package dbmodel

import "gorm.io/gorm"

// CloudRegionDefaults holds user's defaults for a specific region.
type CloudRegionDefaults struct {
	gorm.Model

	UserID string `gorm:"not null;uniqueIndex:idx_defaults_user_id_region_id"`
	User   User   `gorm:"foreignKey:UserID;references:Username"`

	CloudRegionID uint `gorm:"not null;uniqueIndex:idx_defaults_user_id_region_id"`
	CloudRegion   CloudRegion

	Defaults Map
}
