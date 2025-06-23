// Copyright 2025 Canonical.

package db

import (
	"context"
	"errors"
	"fmt"
)

// LockDI represents a unique identifier for an advisory lock in the database.
type LockID int64

const (
	// ControllerBootstrapLock is the advisory lock ID used for controller bootstrap operations.
	ControllerBootstrapLock LockID = 1001
)

// LockAdvisory attempts to acquire an advisory lock with the given ID.
// It returns an error if the lock is already held by another session.
func (d *Database) LockAdvisory(ctx context.Context, id LockID) error {
	var success bool
	err := d.DB.WithContext(ctx).Raw("SELECT pg_try_advisory_lock(?)", id).Scan(&success).Error
	if err != nil {
		return fmt.Errorf("error acquiring advisory lock: %w", err)
	}
	if !success {
		return errors.New("lock is already held")
	}

	return nil
}

// UnlockAdvisory releases an advisory lock with the given ID.
// It returns an error if the lock was not held or could not be released.
func (d *Database) UnlockAdvisory(ctx context.Context, id LockID) error {
	var released bool

	err := d.DB.WithContext(ctx).Raw("SELECT pg_advisory_unlock(?)", id).Scan(&released).Error
	if err != nil {
		return fmt.Errorf("error releasing advisory lock: %w", err)
	}

	if !released {
		return errors.New("failed to release lock, it may not have been held")
	}

	return nil
}
