// Copyright 2021 Canonical Ltd.

package dbmodel

import "gorm.io/gorm"

// UserModelDefaults holds user's model defaults.
type UserModelDefaults struct {
	gorm.Model

	IdentityName string
	Identity     Identity `gorm:"foreignKey:IdentityName;references:Name"`

	Defaults Map
}
