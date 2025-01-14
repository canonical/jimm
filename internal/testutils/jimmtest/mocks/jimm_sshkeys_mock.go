// Copyright 2025 Canonical.
package mocks

import (
	"context"

	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/jimm/sshkeys"
	"github.com/canonical/jimm/v3/internal/openfga"
)

type SSHKeyManager struct {
	AddUserPublicKey_           func(ctx context.Context, user *openfga.User, publicKey sshkeys.PublicKey) error
	ListUserPublicKeys_         func(ctx context.Context, user *openfga.User) ([]sshkeys.PublicKey, error)
	RemoveUserKeyByComment_     func(ctx context.Context, user *openfga.User, comment string) error
	RemoveUserKeyByFingerprint_ func(ctx context.Context, user *openfga.User, fingerprint string) error
}

func (j *SSHKeyManager) AddUserPublicKey(ctx context.Context, user *openfga.User, publicKey sshkeys.PublicKey) error {
	if j.AddUserPublicKey_ == nil {
		return errors.E(errors.CodeNotImplemented)
	}
	return j.AddUserPublicKey_(ctx, user, publicKey)
}
func (j *SSHKeyManager) ListUserPublicKeys(ctx context.Context, user *openfga.User) ([]sshkeys.PublicKey, error) {
	if j.ListUserPublicKeys_ == nil {
		return nil, errors.E(errors.CodeNotImplemented)
	}
	return j.ListUserPublicKeys_(ctx, user)
}
func (j *SSHKeyManager) RemoveUserKeyByComment(ctx context.Context, user *openfga.User, comment string) error {
	if j.RemoveUserKeyByComment_ == nil {
		return errors.E(errors.CodeNotImplemented)
	}
	return j.RemoveUserKeyByComment_(ctx, user, comment)
}
func (j *SSHKeyManager) RemoveUserKeyByFingerprint(ctx context.Context, user *openfga.User, fingerprint string) error {
	if j.RemoveUserKeyByFingerprint_ == nil {
		return errors.E(errors.CodeNotImplemented)
	}
	return j.RemoveUserKeyByFingerprint_(ctx, user, fingerprint)
}
