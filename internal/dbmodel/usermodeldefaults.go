// Copyright 2021 Canonical Ltd.

package dbmodel

import "time"

// UserModelDefaults holds user's model defaults.
type UserModelDefaults struct {
	ID        uint `gorm:"primaryKey"`
	CreatedAt time.Time
	UpdatedAt time.Time

	Username string
	User     User `gorm:"foreignKey:Username;references:Username"`

	Defaults Map
}
