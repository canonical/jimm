// Copyright 2021 Canonical Ltd.

package dbmodel

import "gorm.io/gorm"

// UserModelDefaults holds user's model defaults.
type UserModelDefaults struct {
	gorm.Model

	Username string
	User     User `gorm:"foreignKey:Username;references:Username"`

	Defaults Map
}
