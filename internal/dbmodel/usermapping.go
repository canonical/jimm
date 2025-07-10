// Copyright 2025 Canonical.

package dbmodel

import (
	"database/sql"
	"time"
)

// UserMapping holds information per model on
// mapping local users to external users.
type UserMapping struct {
	// Note this doesn't use the standard gorm.Model to avoid soft-deletes.
	ID        uint `gorm:"primarykey"`
	CreatedAt time.Time
	UpdatedAt time.Time

	// ModelUUID is the UUID of the model the mapping applies to.
	ModelUUID sql.NullString
	Model     Model `gorm:"foreignkey:ModelUUID;references:UUID"`

	// LocalUser is the local user that this mapping applies to.
	LocalUser string

	// ExternalUserName is the external user that this local user maps to.
	ExternalUserName string   `gorm:"column:external_user"`
	ExternalUser     Identity `gorm:"foreignkey:ExternalUserName;references:Name"`
}
