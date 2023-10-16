// Copyright 2021 Canonical Ltd.

package dbmodel

// UserModelDefaults holds user's model defaults.
type UserModelDefaults struct {
	ModelHardDelete

	Username string
	User     User `gorm:"foreignKey:Username;references:Username"`

	Defaults Map
}
