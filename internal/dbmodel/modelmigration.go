// Copyright 2025 Canonical.

package dbmodel

import (
	"database/sql"
	"time"
)

// IncomingModelMigration holds the information for a model migrating into JIMM.
// It includes the model UUID, the target controller for the migration,
// and a mapping of local users to external users that will be persisted
// separately in the UserMapping table if the migration is successful.
type IncomingModelMigration struct {
	// Note this doesn't use the standard gorm.Model to avoid soft-deletes.
	ID        uint `gorm:"primarykey"`
	CreatedAt time.Time
	UpdatedAt time.Time

	// ModelUUID is the UUID of the incoming model.
	ModelUUID sql.NullString

	// TargetControllerID is the target controller for the model undergoing migration.
	TargetControllerID uint
	TargetController   Controller

	// UserMapping holds a mapping of local users to external users.
	UserMapping StringMap
}
