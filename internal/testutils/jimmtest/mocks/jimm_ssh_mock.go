// Copyright 2025 Canonical.

package mocks

import (
	"context"

	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/jimm/ssh"
	"github.com/canonical/jimm/v3/internal/openfga"
)

// SSHManager is an implementation of the SshManager interface.
type SSHManager struct {
	PublicKeyHandler_            func(ctx context.Context, claimUser string, key []byte) (*openfga.User, error)
	ControllerInfoFromModelUUID_ func(ctx context.Context, modelUUID string, user *openfga.User) (ssh.ControllerInfo, error)
}

func (j SSHManager) PublicKeyHandler(ctx context.Context, claimUser string, key []byte) (*openfga.User, error) {
	if j.PublicKeyHandler_ == nil {
		return nil, errors.E(errors.CodeNotImplemented)
	}
	return j.PublicKeyHandler_(ctx, claimUser, key)
}

func (j SSHManager) ControllerInfoFromModelUUID(ctx context.Context, modelUUID string, user *openfga.User) (ssh.ControllerInfo, error) {
	if j.ControllerInfoFromModelUUID_ == nil {
		return ssh.ControllerInfo{}, errors.E(errors.CodeNotImplemented)
	}
	return j.ControllerInfoFromModelUUID_(ctx, modelUUID, user)
}
