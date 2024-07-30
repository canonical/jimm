// Copyright 2024 Canonical Ltd.

package db

import (
	"context"

	"github.com/canonical/jimm/internal/dbmodel"
	"github.com/canonical/jimm/internal/errors"
	"github.com/canonical/jimm/internal/servermon"
)

// AddUserSSHKey adds a user's SSH key to the database.
func (d *Database) AddUserSSHKey(ctx context.Context, userSSHKey *dbmodel.UserSSHKey) (err error) {
	const op = errors.Op("db.AddUserSSHKey")

	if err := d.ready(); err != nil {
		return errors.E(op, err)
	}

	durationObserver := servermon.DurationObserver(servermon.DBQueryDurationHistogram, string(op))
	defer durationObserver()
	defer servermon.ErrorCounter(servermon.DBQueryErrorCount, &err, string(op))

	db := d.DB.WithContext(ctx)
	err = db.Create(userSSHKey).Error
	if err != nil {
		return errors.E(op, dbError(err))
	}
	return nil
}

// DeleteUserSSHKey deletes a user's SSH key from the database.
func (d *Database) DeleteUserSSHKey(ctx context.Context, userSSHKey *dbmodel.UserSSHKey) (err error) {
	const op = errors.Op("db.DeleteUserSSHKey")

	if err := d.ready(); err != nil {
		return errors.E(op, err)
	}

	durationObserver := servermon.DurationObserver(servermon.DBQueryDurationHistogram, string(op))
	defer durationObserver()
	defer servermon.ErrorCounter(servermon.DBQueryErrorCount, &err, string(op))

	db := d.DB.WithContext(ctx)
	err = db.Unscoped().Delete(userSSHKey).Error
	if err != nil {
		return errors.E(op, dbError(err))
	}

	return nil
}

// ListUserSSHKeys returns all the SSH keys that belong to a use. You need only populate
// the Name field of the identity argument.
func (d *Database) ListUserSSHKeys(ctx context.Context, identity *dbmodel.Identity) (sshKeys []string, err error) {
	const op = errors.Op("db.DeleteUserSSHKey")

	if err := d.ready(); err != nil {
		return sshKeys, errors.E(op, err)
	}

	durationObserver := servermon.DurationObserver(servermon.DBQueryDurationHistogram, string(op))
	defer durationObserver()
	defer servermon.ErrorCounter(servermon.DBQueryErrorCount, &err, string(op))

	db := d.DB.WithContext(ctx)

	err = db.Model(&dbmodel.UserSSHKey{}).
		Where("identity_name = ?", identity.Name).
		Pluck("ssh_key", &sshKeys).Error
	if err != nil {
		return sshKeys, errors.E(op, dbError(err))
	}
	return
}
