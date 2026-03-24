// Copyright 2025 Canonical.

package mocks

import (
	"context"

	gossh "golang.org/x/crypto/ssh"

	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/jimm/ssh"
	"github.com/canonical/jimm/v3/internal/openfga"
)

// SSHManager is an implementation of the SshManager interface.
type SSHManager struct {
	PublicKeyHandler_ func(ctx context.Context, claimUser string, key []byte) (*openfga.User, error)
	DialController_   func(ctx context.Context, ctrlInfo ssh.DialInfo, user *openfga.User) (*gossh.Client, error)
	DialInfo_         func(ctx context.Context, modelUUID string, user *openfga.User) (ssh.DialInfo, error)
}

func (j SSHManager) PublicKeyHandler(ctx context.Context, claimUser string, key []byte) (*openfga.User, error) {
	if j.PublicKeyHandler_ == nil {
		return nil, errors.New("not implemented")
	}
	return j.PublicKeyHandler_(ctx, claimUser, key)
}

func (j SSHManager) DialController(ctx context.Context, ctrlInfo ssh.DialInfo, user *openfga.User) (*gossh.Client, error) {
	if j.DialController_ == nil {
		return nil, errors.New("not implemented")
	}
	return j.DialController_(ctx, ctrlInfo, user)
}

func (j SSHManager) DialInfo(ctx context.Context, modelUUID string, user *openfga.User) (ssh.DialInfo, error) {
	if j.DialInfo_ == nil {
		return ssh.DialInfo{}, errors.New("not implemented")
	}
	return j.DialInfo_(ctx, modelUUID, user)
}
