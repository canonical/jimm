// Copyright 2021 Canonical Ltd.

package dbmodel

import "time"

// CloudDefaults holds user's defaults for a specific cloud/region.
type CloudDefaults struct {
	ID        uint `gorm:"primaryKey"`
	CreatedAt time.Time
	UpdatedAt time.Time

	Username string
	User     User `gorm:"foreignKey:Username;references:Username"`

	CloudID uint
	Cloud   Cloud

	Region string

	Defaults Map
}
