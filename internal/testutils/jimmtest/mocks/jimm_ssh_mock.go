// Copyright 2025 Canonical.

package mocks

import (
	"context"

	"github.com/juju/names/v5"

	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/openfga"
)

// SSHResolver is an implementation of the sshResolver interface.
type SSHResolver struct {
	AddrFromModelUUID_ func(ctx context.Context, user *openfga.User, modelTag names.ModelTag) (string, error)
}

func (j SSHResolver) AddrFromModelUUID(ctx context.Context, user *openfga.User, modelTag names.ModelTag) (string, error) {
	if j.AddrFromModelUUID_ == nil {
		return "", errors.E(errors.CodeNotImplemented)
	}
	return j.AddrFromModelUUID_(ctx, user, modelTag)
}

// SSHResolver is an implementation of the sshResolver interface.
type SSHAuthorizer struct {
	PublicKeyHandler_ func(ctx context.Context, claimUser string, key []byte) (*openfga.User, error)
}

func (j SSHAuthorizer) PublicKeyHandler(ctx context.Context, claimUser string, key []byte) (*openfga.User, error) {
	if j.PublicKeyHandler_ == nil {
		return nil, errors.E(errors.CodeNotImplemented)
	}
	return j.PublicKeyHandler_(ctx, claimUser, key)
}
