// Copyright 2021 Canonical Ltd.

package dbmodel

import "gorm.io/gorm"

// CloudDefaults holds user's defaults for a specific cloud/region.
type CloudDefaults struct {
	gorm.Model

	UserID string `gorm:"not null;uniqueIndex:idx_defaults_user_id_cloud_id_region"`
	User   User   `gorm:"foreignKey:UserID;references:Username"`

	CloudID uint `gorm:"not null;uniqueIndex:idx_defaults_user_id_cloud_id_region"`
	Cloud   Cloud

	Region string `gorm:"uniqueIndex:idx_defaults_user_id_cloud_id_region"`

	Defaults Map
}
