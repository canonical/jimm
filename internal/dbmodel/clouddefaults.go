// Copyright 2021 Canonical Ltd.

package dbmodel

// CloudDefaults holds user's defaults for a specific cloud/region.
type CloudDefaults struct {
	ModelHardDelete

	Username string
	User     User `gorm:"foreignKey:Username;references:Username"`

	CloudID uint
	Cloud   Cloud

	Region string

	Defaults Map
}
