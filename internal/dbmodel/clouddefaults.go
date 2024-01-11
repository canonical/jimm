// Copyright 2021 Canonical Ltd.

package dbmodel

import "gorm.io/gorm"

// CloudDefaults holds user's defaults for a specific cloud/region.
type CloudDefaults struct {
	gorm.Model

	Username string
	User     Identity `gorm:"foreignKey:Username;references:Username"`

	CloudID uint
	Cloud   Cloud

	Region string

	Defaults Map
}
