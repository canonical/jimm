// Copyright 2025 Canonical.

package mocks

import (
	"context"

	"github.com/canonical/jimm/v3/internal/db"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/jimm/sshkeys"
	"github.com/canonical/jimm/v3/internal/openfga"
)

type SSHKeyManager struct {
	AddUserPublicKey_   func(ctx context.Context, user *openfga.User, model db.SSHKeyModelFilter, publicKey sshkeys.PublicKey) error
	ListUserPublicKeys_ func(ctx context.Context, user *openfga.User, model db.SSHKeyModelFilter) ([]sshkeys.PublicKey, error)
	RemoveUserKeys_     func(ctx context.Context, user *openfga.User, model db.SSHKeyModelFilter, targets ...string) error
	VerifyPublicKey_    func(ctx context.Context, claimUser string, publicKey []byte) (bool, error)
}

func (j *SSHKeyManager) AddUserPublicKey(ctx context.Context, user *openfga.User, model db.SSHKeyModelFilter, publicKey sshkeys.PublicKey) error {
	if j.AddUserPublicKey_ == nil {
		return errors.New("not implemented")
	}
	return j.AddUserPublicKey_(ctx, user, model, publicKey)
}

func (j *SSHKeyManager) ListUserPublicKeys(ctx context.Context, user *openfga.User, model db.SSHKeyModelFilter) ([]sshkeys.PublicKey, error) {
	if j.ListUserPublicKeys_ == nil {
		return nil, errors.New("not implemented")
	}
	return j.ListUserPublicKeys_(ctx, user, model)
}

func (j *SSHKeyManager) RemoveUserKeys(ctx context.Context, user *openfga.User, model db.SSHKeyModelFilter, targets ...string) error {
	if j.RemoveUserKeys_ == nil {
		return errors.New("not implemented")
	}
	return j.RemoveUserKeys_(ctx, user, model, targets...)
}

func (j *SSHKeyManager) VerifyPublicKey(ctx context.Context, claimUser string, publicKey []byte) (bool, error) {
	if j.VerifyPublicKey_ == nil {
		return false, errors.New("not implemented")
	}
	return j.VerifyPublicKey_(ctx, claimUser, publicKey)
}
