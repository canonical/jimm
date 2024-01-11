// Copyright 2021 Canonical Ltd.

package dbmodel

import "gorm.io/gorm"

// CloudDefaults holds user's defaults for a specific cloud/region.
type CloudDefaults struct {
	gorm.Model

	IdentityName string
	User         Identity `gorm:"foreignKey:IdentityName;references:Name"`

	CloudID uint
	Cloud   Cloud

	Region string

	Defaults Map
}
