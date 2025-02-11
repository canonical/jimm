// Copyright 2025 Canonical.

package mocks

import (
	"context"

	gossh "golang.org/x/crypto/ssh"

	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/openfga"
)

// SSHManager is an implementation of the SshManager interface.
type SSHManager struct {
	PublicKeyHandler_        func(ctx context.Context, claimUser string, key []byte) (*openfga.User, error)
	DialControllerSSHServer_ func(ctx context.Context, modelUUID string, user *openfga.User) (*gossh.Client, error)
}

func (j SSHManager) PublicKeyHandler(ctx context.Context, claimUser string, key []byte) (*openfga.User, error) {
	if j.PublicKeyHandler_ == nil {
		return nil, errors.E(errors.CodeNotImplemented)
	}
	return j.PublicKeyHandler_(ctx, claimUser, key)
}

func (j SSHManager) DialControllerSSHServer(ctx context.Context, modelUUID string, user *openfga.User) (*gossh.Client, error) {
	if j.DialControllerSSHServer_ == nil {
		return nil, errors.E(errors.CodeNotImplemented)
	}
	return j.DialControllerSSHServer_(ctx, modelUUID, user)
}
