// Copyright 2025 Canonical.

package db

import (
	"context"
	"fmt"

	gossh "golang.org/x/crypto/ssh"

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
		return errors.E(op, dbError(err))
	}
	return nil
}

// RemoveSSHKeyByFingerprint removes a user's ssh key identified by its fingerprint.
func (d *Database) RemoveSSHKeyByFingerprint(ctx context.Context, identityName string, fingerprint string) (err error) {
	const op = errors.Op("db.RemoveSSHKeyByFingerprint")

	if err := d.ready(); err != nil {
		return errors.E(op, err)
	}

	durationObserver := servermon.DurationObserver(servermon.DBQueryDurationHistogram, string(op))
	defer durationObserver()
	defer servermon.ErrorCounter(servermon.DBQueryErrorCount, &err, string(op))

	var keys []dbmodel.SSHKey
	if err := d.DB.Where("identity_name = ?", identityName).Find(&keys).Error; err != nil {
		return errors.E(op, dbError(err))
	}

	// It is expected that we only have 1 key that matches this fingerprint
	// because of the unique constraint between users and public keys.
	for _, key := range keys {
		fp, err := calculateSSHFingerprint(key.PublicKey)
		if err != nil {
			return errors.E(op, err)
		}
		if fp == fingerprint {
			if err := d.DB.WithContext(ctx).Delete(key).Error; err != nil {
				return errors.E(op, dbError(err))
			}
			return nil
		}
	}

	return errors.E(op, errors.CodeNotFound, "key not found")
}

// RemoveSSHKeyByComment removes a user's ssh key identified by its comment.
func (d *Database) RemoveSSHKeyByComment(ctx context.Context, identityName string, comment string) (err error) {
	const op = errors.Op("db.RemoveSSHKeyByComment")

	if err := d.ready(); err != nil {
		return errors.E(op, err)
	}

	durationObserver := servermon.DurationObserver(servermon.DBQueryDurationHistogram, string(op))
	defer durationObserver()
	defer servermon.ErrorCounter(servermon.DBQueryErrorCount, &err, string(op))

	query := d.DB.Where("key_comment = ?", comment).Delete(&dbmodel.SSHKey{})
	if err := query.Error; err != nil {
		return errors.E(op, dbError(err))
	}

	if query.RowsAffected == 0 {
		return errors.E(op, errors.CodeNotFound, "key not found")
	}

	return nil
}

// ListSSHKeys all a user's SSH keys.
func (d *Database) ListSSHKeys(ctx context.Context, identityName string) (keys []dbmodel.SSHKey, err error) {
	const op = errors.Op("db.ListSSHKeys")

	if err := d.ready(); err != nil {
		return nil, errors.E(op, err)
	}

	durationObserver := servermon.DurationObserver(servermon.DBQueryDurationHistogram, string(op))
	defer durationObserver()
	defer servermon.ErrorCounter(servermon.DBQueryErrorCount, &err, string(op))

	if err := d.DB.Where("identity_name = ?", identityName).Find(&keys).Error; err != nil {
		return nil, errors.E(op, dbError(err))
	}

	return keys, nil
}

// calculateSSHFingerprint parses an SSH public key and returns its MD5 fingerprint
func calculateSSHFingerprint(publicKey []byte) (string, error) {
	parsedKey, err := gossh.ParsePublicKey(publicKey)
	if err != nil {
		return "", fmt.Errorf("invalid SSH key: %v", err)
	}

	return gossh.FingerprintLegacyMD5(parsedKey), nil
}
