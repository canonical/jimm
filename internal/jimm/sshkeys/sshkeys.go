// Copyright 2025 Canonical.

package sshkeys

import (
	"context"

	gossh "golang.org/x/crypto/ssh"

	"github.com/canonical/jimm/v3/internal/db"
	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/openfga"
)

type sshKeyManager struct {
	store *db.Database
}

// NewSSHKeyManager returns a new sshKeyManager that handles ssh keys.
func NewSSHKeyManager(store *db.Database) (*sshKeyManager, error) {
	if store == nil {
		return nil, errors.E("role store cannot be nil")
	}
	return &sshKeyManager{store}, nil
}

// AddUserPublicKey saves a user's public key.
func (rm *sshKeyManager) AddUserPublicKey(ctx context.Context, user *openfga.User, publicKey PublicKey) error {
	const op = errors.Op("sshkeys.AddUserPublicKey")

	if ok, reason := publicKey.valid(); !ok {
		return errors.E(op, errors.CodeBadRequest, reason)
	}

	k := dbmodel.SSHKey{
		IdentityName:   user.Name,
		PublicKey:      publicKey.Marshal(),
		MD5Fingerprint: gossh.FingerprintLegacyMD5(publicKey),
		KeyComment:     publicKey.Comment,
	}
	err := rm.store.AddSSHKey(ctx, &k)
	if err != nil {
		return errors.E(op, err)
	}
	return nil
}

// ListUserPublicKeys lists a user's public keys.
func (rm *sshKeyManager) ListUserPublicKeys(ctx context.Context, user *openfga.User) ([]PublicKey, error) {
	const op = errors.Op("sshkeys.ListUserPublicKeys")

	dbKeys, err := rm.store.ListSSHKeysForUser(ctx, user.Name)
	if err != nil {
		return nil, errors.E(op, err)
	}
	var pubKeys []PublicKey
	for _, key := range dbKeys {
		k, err := gossh.ParsePublicKey(key.PublicKey)
		if err != nil {
			return nil, errors.E(op, err)
		}
		pubKeys = append(pubKeys, PublicKey{PublicKey: k, Comment: key.KeyComment})
	}
	return pubKeys, nil
}

// RemoveUserKeyByComment removes a user's public key(s) by the key comment.
func (rm *sshKeyManager) RemoveUserKeyByComment(ctx context.Context, user *openfga.User, comment string) error {
	const op = errors.Op("sshkeys.RemoveUserKeyByComment")

	err := rm.store.RemoveSSHKeyByComment(ctx, user.Name, comment)
	if err != nil {
		return errors.E(op, err)
	}
	return nil
}

// RemoveUserKeyByFingerprint removes a user's public key by the key fingerprint.
func (rm *sshKeyManager) RemoveUserKeyByFingerprint(ctx context.Context, user *openfga.User, fingerprint string) error {
	const op = errors.Op("sshkeys.RemoveUserKeyByFingerprint")

	err := rm.store.RemoveSSHKeyByFingerprint(ctx, user.Name, fingerprint)
	if err != nil {
		return errors.E(op, err)
	}
	return nil
}
