// Copyright 2021 Canonical Ltd.

package dbmodel

import "gorm.io/gorm"

// UserModelDefaults holds user's model defaults.
type UserModelDefaults struct {
	gorm.Model

	UserID string `gorm:"not null;uniqueIndex:idx_controller_defaults_controller_id_user_id"`
	User   User   `gorm:"foreignKey:UserID;references:Username"`

	Defaults Map
}
