// Copyright 2025 Canonical.

package mocks

import (
	"context"

	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/openfga"
)

// SSHManager is an implementation of the SshManager interface.
type SSHManager struct {
	PublicKeyHandler_              func(ctx context.Context, claimUser string, key []byte) (*openfga.User, error)
	ResolveAddressesFromModelUUID_ func(ctx context.Context, modelUUID string) ([]string, error)
}

func (j SSHManager) PublicKeyHandler(ctx context.Context, claimUser string, key []byte) (*openfga.User, error) {
	if j.PublicKeyHandler_ == nil {
		return nil, errors.E(errors.CodeNotImplemented)
	}
	return j.PublicKeyHandler_(ctx, claimUser, key)
}

func (j SSHManager) ResolveAddressesFromModelUUID(ctx context.Context, modelUUID string) ([]string, error) {
	if j.ResolveAddressesFromModelUUID_ == nil {
		return nil, errors.E(errors.CodeNotImplemented)
	}
	return j.ResolveAddressesFromModelUUID_(ctx, modelUUID)
}
