// Copyright 2025 Canonical.

package db

import (
	"context"

	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/servermon"
)

// AddSSHKey adds a new SSH key.
func (d *Database) AddSSHKey(ctx context.Context, sshKey *dbmodel.SSHKey) (err error) {
	const op = errors.Op("db.AddSSHKey")
	if err := d.ready(); err != nil {
		return errors.E(op, err)
	}

	durationObserver := servermon.DurationObserver(servermon.DBQueryDurationHistogram, string(op))
	defer durationObserver()
	defer servermon.ErrorCounter(servermon.DBQueryErrorCount, &err, string(op))

	if err := d.DB.WithContext(ctx).Create(sshKey).Error; err != nil {
		dbErr := dbError(err)
		if errors.ErrorCode(dbErr) == errors.CodeAlreadyExists {
			// we don't return an error if a user tries to add the same key twice.
			return nil
		}
		return errors.E(op, dbErr)
	}
	return nil
}

// RemoveSSHKeyByFingerprint removes a user's ssh key identified by its fingerprint.
func (d *Database) RemoveSSHKeyByFingerprint(ctx context.Context, identityName string, model SSHKeyModelFilter, fingerprint string) (err error) {
	const op = errors.Op("db.RemoveSSHKeyByFingerprint")

	if err := d.ready(); err != nil {
		return errors.E(op, err)
	}

	durationObserver := servermon.DurationObserver(servermon.DBQueryDurationHistogram, string(op))
	defer durationObserver()
	defer servermon.ErrorCounter(servermon.DBQueryErrorCount, &err, string(op))

	query := d.DB.Where("identity_name = ?", identityName).
		Where("model_uuid = ?", model.ModelUUID).
		Where("md5_fingerprint = ?", fingerprint).
		Delete(&dbmodel.SSHKey{})

	if err := query.Error; err != nil {
		return errors.E(op, dbError(err))
	}

	if query.RowsAffected == 0 {
		return errors.E(op, errors.CodeNotFound, "key not found")
	}

	return nil
}

// RemoveSSHKeyByComment removes a user's ssh key identified by its comment.
func (d *Database) RemoveSSHKeyByComment(ctx context.Context, identityName string, model SSHKeyModelFilter, comment string) (err error) {
	const op = errors.Op("db.RemoveSSHKeyByComment")

	if err := d.ready(); err != nil {
		return errors.E(op, err)
	}

	durationObserver := servermon.DurationObserver(servermon.DBQueryDurationHistogram, string(op))
	defer durationObserver()
	defer servermon.ErrorCounter(servermon.DBQueryErrorCount, &err, string(op))

	query := d.DB.Where("key_comment = ?", comment).
		Where("model_uuid = ?", model.ModelUUID).
		Delete(&dbmodel.SSHKey{})
	if err := query.Error; err != nil {
		return errors.E(op, dbError(err))
	}

	if query.RowsAffected == 0 {
		return errors.E(op, errors.CodeNotFound, "key not found")
	}

	return nil
}

// ListSSHKeysForUser lists all user's SSH keys per model.
func (d *Database) ListSSHKeysForUser(ctx context.Context, identityName string, model SSHKeyModelFilter) (keys []dbmodel.SSHKey, err error) {
	const op = errors.Op("db.ListSSHKeysForUser")

	if err := d.ready(); err != nil {
		return nil, errors.E(op, err)
	}

	durationObserver := servermon.DurationObserver(servermon.DBQueryDurationHistogram, string(op))
	defer durationObserver()
	defer servermon.ErrorCounter(servermon.DBQueryErrorCount, &err, string(op))
	query := d.DB.Where("identity_name = ?", identityName)
	if !model.All {
		query = query.Where("model_uuid = ?", model.ModelUUID)
	}
	if err := query.
		Find(&keys).Error; err != nil {
		return nil, errors.E(op, dbError(err))
	}

	return keys, nil
}

// SSHKeyModelFilter holds the model UUID for the SSH Key or a flag to list all keys independent of the model.
type SSHKeyModelFilter struct {
	ModelUUID string
	All       bool
}
