// Copyright 2025 Canonical.

package dbmodel

import (
	"database/sql"
	"time"
)

// ModelMigration holds the information for a pending model migration.
type ModelMigration struct {
	// Note this doesn't use the standard gorm.Model to avoid soft-deletes.
	ID        uint `gorm:"primarykey"`
	CreatedAt time.Time

	// ModelUUID is the UUID of the incoming model.
	ModelUUID sql.NullString

	// TargetControllerID is the target controller for the model undegroing migration.
	TargetControllerID uint
	TargetController   Controller

	// UserMapping holds a mapping of local users to external users.
	UserMapping JSON
}
