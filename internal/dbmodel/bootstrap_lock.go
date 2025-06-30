// Copyright 2025 Canonical.

package dbmodel

import "time"

// BootstrapLock represents a lock for controller bootstrap operations.
type BootstrapLock struct {
	ID uint `gorm:"primarykey"`

	ExpiresAt time.Time
	Locked    bool
}
