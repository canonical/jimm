// Copyright 2025 Canonical.

package sshkeys

import (
	"context"
	"fmt"

	"github.com/gliderlabs/ssh"
	gossh "golang.org/x/crypto/ssh"

	"github.com/canonical/jimm/v3/internal/db"
	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/openfga"
)

type SSHKeyManager struct {
	store *db.Database
}

// NewSSHKeyManager returns a new sshKeyManager that handles ssh keys.
func NewSSHKeyManager(store *db.Database) (*SSHKeyManager, error) {
	if store == nil {
		return nil, errors.E("role store cannot be nil")
	}
	return &SSHKeyManager{store}, nil
}

// AddUserPublicKey saves a user's public key.
func (sm *SSHKeyManager) AddUserPublicKey(ctx context.Context, user *openfga.User, model db.SSHKeyModelFilter, publicKey PublicKey) error {

	if ok, reason := publicKey.valid(); !ok {
		return errors.E(errors.CodeBadRequest, reason)
	}

	k := dbmodel.SSHKey{
		IdentityName:   user.Name,
		ModelUUID:      model.ModelUUID,
		PublicKey:      publicKey.Marshal(),
		MD5Fingerprint: gossh.FingerprintLegacyMD5(publicKey),
		KeyComment:     publicKey.Comment,
	}
	err := sm.store.AddSSHKey(ctx, &k)
	if err != nil {
		return err
	}
	return nil
}

// VerifyPublicKey lists the key for a user and compares the key to find a match.
func (sm *SSHKeyManager) VerifyPublicKey(ctx context.Context, claimUser string, publicKey []byte) (bool, error) {

	dbKeys, err := sm.store.ListSSHKeysForUser(ctx, claimUser, db.SSHKeyModelFilter{All: true})
	if err != nil {
		return false, err
	}
	publicKeyToCompare, err := gossh.ParsePublicKey(publicKey)
	if err != nil {
		return false, err
	}
	for _, key := range dbKeys {
		k, err := gossh.ParsePublicKey(key.PublicKey)
		if err != nil {
			return false, err
		}
		if ssh.KeysEqual(k, publicKeyToCompare) {
			return true, nil
		}
	}
	return false, fmt.Errorf("cannot find a matching key for this user")

}

// ListUserPublicKeys lists a user's public keys.
func (sm *SSHKeyManager) ListUserPublicKeys(ctx context.Context, user *openfga.User, model db.SSHKeyModelFilter) ([]PublicKey, error) {

	dbKeys, err := sm.store.ListSSHKeysForUser(ctx, user.Name, model)
	if err != nil {
		return nil, err
	}
	var pubKeys []PublicKey
	for _, key := range dbKeys {
		k, err := gossh.ParsePublicKey(key.PublicKey)
		if err != nil {
			return nil, err
		}
		pubKeys = append(pubKeys, PublicKey{PublicKey: k, Comment: key.KeyComment})
	}
	return pubKeys, nil
}

// RemoveUserKeyByComment removes a user's public key(s) by the key comment.
func (sm *SSHKeyManager) RemoveUserKeyByComment(ctx context.Context, user *openfga.User, model db.SSHKeyModelFilter, comment string) error {

	err := sm.store.RemoveSSHKeyByComment(ctx, user.Name, model, comment)
	if err != nil {
		return err
	}
	return nil
}

// RemoveUserKeyByFingerprint removes a user's public key by the key fingerprint.
func (sm *SSHKeyManager) RemoveUserKeyByFingerprint(ctx context.Context, user *openfga.User, model db.SSHKeyModelFilter, fingerprint string) error {

	err := sm.store.RemoveSSHKeyByFingerprint(ctx, user.Name, model, fingerprint)
	if err != nil {
		return err
	}
	return nil
}
