package mocks

import (
	"context"

	"github.com/canonical/jimm/v3/internal/db"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/jimm/sshkeys"
	"github.com/canonical/jimm/v3/internal/openfga"
)

type SSHKeyManager struct {
	AddUserPublicKey_           func(ctx context.Context, user *openfga.User, model db.SSHKeyModelFilter, publicKey sshkeys.PublicKey) error
	ListUserPublicKeys_         func(ctx context.Context, user *openfga.User, model db.SSHKeyModelFilter) ([]sshkeys.PublicKey, error)
	RemoveUserKeyByComment_     func(ctx context.Context, user *openfga.User, model db.SSHKeyModelFilter, comment string) error
	RemoveUserKeyByFingerprint_ func(ctx context.Context, user *openfga.User, model db.SSHKeyModelFilter, fingerprint string) error
	VerifyPublicKey_            func(ctx context.Context, claimUser string, publicKey []byte) (bool, error)
}

func (j *SSHKeyManager) AddUserPublicKey(ctx context.Context, user *openfga.User, model db.SSHKeyModelFilter, publicKey sshkeys.PublicKey) error {
	if j.AddUserPublicKey_ == nil {
		return errors.E(errors.CodeNotImplemented)
	}
	return j.AddUserPublicKey_(ctx, user, model, publicKey)
}

func (j *SSHKeyManager) ListUserPublicKeys(ctx context.Context, user *openfga.User, model db.SSHKeyModelFilter) ([]sshkeys.PublicKey, error) {
	if j.ListUserPublicKeys_ == nil {
		return nil, errors.E(errors.CodeNotImplemented)
	}
	return j.ListUserPublicKeys_(ctx, user, model)
}

func (j *SSHKeyManager) RemoveUserKeyByComment(ctx context.Context, user *openfga.User, model db.SSHKeyModelFilter, comment string) error {
	if j.RemoveUserKeyByComment_ == nil {
		return errors.E(errors.CodeNotImplemented)
	}
	return j.RemoveUserKeyByComment_(ctx, user, model, comment)
}

func (j *SSHKeyManager) RemoveUserKeyByFingerprint(ctx context.Context, user *openfga.User, model db.SSHKeyModelFilter, fingerprint string) error {
	if j.RemoveUserKeyByFingerprint_ == nil {
		return errors.E(errors.CodeNotImplemented)
	}
	return j.RemoveUserKeyByFingerprint_(ctx, user, model, fingerprint)
}

func (j *SSHKeyManager) VerifyPublicKey(ctx context.Context, claimUser string, publicKey []byte) (bool, error) {
	if j.VerifyPublicKey_ == nil {
		return false, errors.E(errors.CodeNotImplemented)
	}
	return j.VerifyPublicKey_(ctx, claimUser, publicKey)
}
