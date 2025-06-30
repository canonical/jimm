// Copyright 2025 Canonical.

package db

import (
	"context"
	goerr "errors"
	"fmt"
	"time"

	"gorm.io/gorm/clause"

	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
)

// LockBootstrap acquires the bootstrap lock for controller bootstrap operations.
// It returns an error if the lock cannot be acquired.
func (d *Database) LockBootstrap(ctx context.Context, ttl time.Duration) error {
	op := errors.Op("db.LockBootstrap")
	err := d.Transaction(func(tx *Database) error {
		lock := &dbmodel.BootstrapLock{}
		// This is equivalent to a SELECT FOR UPDATE in SQL,
		// which locks the row for the duration of the transaction.
		// The other select statements will wait until this lock is released.
		if err := tx.DB.Clauses(clause.Locking{Strength: "UPDATE"}).First(lock).Error; err != nil {
			return err
		}
		if !lock.Locked || lock.ExpiresAt.Before(time.Now()) {
			lock.Locked = true
			lock.ExpiresAt = time.Now().Add(ttl)
			if err := tx.DB.Save(lock).Error; err != nil {
				return fmt.Errorf("failed to acquire bootstrap lock: %w", err)
			}
		} else {
			return goerr.New("bootstrap lock is already held")
		}
		return nil
	})
	if err != nil {
		return errors.E(op, err)
	}
	return nil
}

// UnlockBootstrap releases the bootstrap lock for controller bootstrap operations.
// It returns an error if the lock was not held or could not be released.
func (d *Database) UnlockBootstrap(ctx context.Context) error {
	op := errors.Op("db.LockBootstrap")
	err := d.Transaction(func(tx *Database) error {
		lock := &dbmodel.BootstrapLock{}
		if err := tx.DB.First(lock).Error; err != nil {
			return fmt.Errorf("failed to find bootstrap lock: %w", err)
		}
		if !lock.Locked {
			return goerr.New("bootstrap lock is not held")
		}
		lock.Locked = false
		lock.ExpiresAt = time.Time{} // Reset the expire time
		if err := tx.DB.Save(lock).Error; err != nil {
			return fmt.Errorf("failed to release bootstrap lock: %w", err)
		}
		return nil
	})
	if err != nil {
		return errors.E(op, err)
	}
	return nil
}
