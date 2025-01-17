// Copyright 2025 Canonical.

package ssh

import (
	"context"
	"fmt"

	"github.com/juju/zaputil/zapctx"

	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/openfga"
)

// IdentityManager provides a means to fetch an identity from the identity service.
type IdentityManager interface {
	FetchIdentity(ctx context.Context, id string) (*openfga.User, error)
}

// ModelManager provides a means to fetch a model from the model service.
type ModelManager interface {
	GetModel(ctx context.Context, uuid string) (dbmodel.Model, error)
}

// SSHKeyManager provides a means to manage ssh keys within JIMM.
type SSHKeyManager interface {
	VerifyPublicKey(ctx context.Context, claimUser string, publicKey []byte) (bool, error)
}

// NewSSHManager returns a new SSHManager that offers jimm functionality to the SSHJumpServer.
func NewSSHManager(identityManager IdentityManager, modelManager ModelManager, sshKeyManager SSHKeyManager) (*sshManager, error) {
	if identityManager == nil {
		return nil, errors.E("identityManager cannot be nil")
	}
	if modelManager == nil {
		return nil, errors.E("modelManager cannot be nil")
	}
	if sshKeyManager == nil {
		return nil, errors.E("sshManager cannot be nil")
	}
	return &sshManager{
		modelManager:    modelManager,
		identityManager: identityManager,
		sshKeyManager:   sshKeyManager,
	}, nil
}

// sshManager provides a means to manage ssh server within JIMM.
type sshManager struct {
	modelManager    ModelManager
	identityManager IdentityManager
	sshKeyManager   SSHKeyManager
}

// PublicKeyHandler is the method to verify the public key of the user. It first checks for the public key and then fetches the user.
// It returns a user if successful.
func (s *sshManager) PublicKeyHandler(ctx context.Context, claimUser string, key []byte) (*openfga.User, error) {
	zapctx.Info(ctx, "PublicKeyHandler")
	if ok, err := s.sshKeyManager.VerifyPublicKey(ctx, claimUser, key); !ok || err != nil {
		return nil, fmt.Errorf("cannot verify key for user %s: %s", claimUser, err.Error())
	}
	user, err := s.identityManager.FetchIdentity(ctx, claimUser)
	if err != nil {
		zapctx.Info(ctx, fmt.Sprintf("cannot find user %s", claimUser))
		return nil, fmt.Errorf("cannot find user %s: %s", claimUser, err.Error())
	}
	return user, nil
}
