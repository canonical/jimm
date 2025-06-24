// Copyright 2025 Canonical.

package db

import (
	"context"
	"errors"
	"fmt"
)

// lockID represents a unique identifier for an advisory lock in the database.
type lockID int64

const (
	// The following IDs should NEVER under any circumstance be changed.
	// If changed, thou be in deep trouble from the Git Blame god!

	// controllerBootstrapLock is the advisory lock ID used for controller bootstrap operations.
	controllerBootstrapLock lockID = iota + 1
)

// lockAdvisory attempts to acquire an advisory lock with the given ID.
// It returns an error if the lock is already held by another session.
func (d *Database) lockAdvisory(ctx context.Context, id lockID) error {
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

// unlockAdvisory releases an advisory lock with the given ID.
// It returns an error if the lock was not held or could not be released.
func (d *Database) unlockAdvisory(ctx context.Context, id lockID) error {
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

// LockBootstrap acquires the advisory lock for controller bootstrap operations.
// It returns an error if the lock cannot be acquired.
func (d *Database) LockBootstrap(ctx context.Context) error {
	return d.lockAdvisory(ctx, controllerBootstrapLock)
}

// UnlockBootstrap releases the advisory lock for controller bootstrap operations.
// It returns an error if the lock was not held or could not be released.
func (d *Database) UnlockBootstrap(ctx context.Context) error {
	return d.unlockAdvisory(ctx, controllerBootstrapLock)
}
