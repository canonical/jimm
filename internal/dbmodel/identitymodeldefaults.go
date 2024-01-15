// Copyright 2021 Canonical Ltd.

package dbmodel

import "gorm.io/gorm"

// IdentityModelDefaults holds identities's model defaults.
type IdentityModelDefaults struct {
	gorm.Model

	IdentityName string
	Identity     Identity `gorm:"foreignKey:IdentityName;references:Name"`

	Defaults Map
}
