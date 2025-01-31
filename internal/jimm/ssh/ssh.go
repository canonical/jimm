// Copyright 2025 Canonical.

package ssh

import (
	"context"
	"fmt"

	"github.com/juju/zaputil/zapctx"

	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/jimm/jujuauth"
	"github.com/canonical/jimm/v3/internal/openfga"
	"github.com/canonical/jimm/v3/internal/rpc"
)

// ControllerInfo is the struct holding the infomation to contact a controller
type ControllerInfo struct {
	// addresses to dial the controller
	Addresses []string
	// JWT to authenticate to the controller
	JWT string
}

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

// JWTGeneratorFactory provides a means to create a token generator.
type JWTGeneratorFactory interface {
	New() jujuauth.TokenGenerator
}

// NewSSHManager returns a new SSHManager that offers jimm functionality to the SSHJumpServer.
func NewSSHManager(identityManager IdentityManager, modelManager ModelManager, sshKeyManager SSHKeyManager, jwtFactory JWTGeneratorFactory) (*sshManager, error) {
	if identityManager == nil {
		return nil, errors.E("identityManager cannot be nil")
	}
	if modelManager == nil {
		return nil, errors.E("modelManager cannot be nil")
	}
	if sshKeyManager == nil {
		return nil, errors.E("sshManager cannot be nil")
	}
	if jwtFactory == nil {
		return nil, errors.E("jwtFactory cannot be nil")
	}
	return &sshManager{
		modelManager:    modelManager,
		identityManager: identityManager,
		sshKeyManager:   sshKeyManager,
		jwtFactory:      jwtFactory,
	}, nil
}

// sshManager provides a means to manage ssh server within JIMM.
type sshManager struct {
	modelManager    ModelManager
	identityManager IdentityManager
	sshKeyManager   SSHKeyManager
	jwtFactory      JWTGeneratorFactory
}

// PublicKeyHandler is the method to verify the public key of the user. It first checks for the public key and then fetches the user.
// It returns a user if successful.
func (s *sshManager) PublicKeyHandler(ctx context.Context, claimUser string, key []byte) (*openfga.User, error) {
	zapctx.Info(ctx, "PublicKeyHandler")
	if ok, err := s.sshKeyManager.VerifyPublicKey(ctx, claimUser, key); !ok || err != nil {
		return nil, errors.E(err, "cannot verify key for user")
	}
	user, err := s.identityManager.FetchIdentity(ctx, claimUser)
	if err != nil {
		zapctx.Info(ctx, fmt.Sprintf("cannot find user %s", claimUser))
		return nil, errors.E(err, "cannot find user")
	}
	return user, nil
}

// ControllerInfoFromModelUUID is the method to resolve the address of the controller to contact given the model UUID and
// a valid JWT To connect to the controller.
func (s *sshManager) ControllerInfoFromModelUUID(ctx context.Context, modelUUID string, user *openfga.User) (ControllerInfo, error) {
	zapctx.Info(ctx, "ControllerInfoFromModelUUID")
	model, err := s.modelManager.GetModel(ctx, modelUUID)
	if err != nil {
		return ControllerInfo{}, errors.E(err, "cannot find model")
	}
	addrs, _ := rpc.GetAddressesAndTLSConfig(ctx, &model.Controller)
	if len(addrs) == 0 {
		return ControllerInfo{}, errors.E(err, "cannot find addresses for model's controller")
	}
	jwtGenerator := s.jwtFactory.New()
	jwtGenerator.SetTags(model.ResourceTag(), model.Controller.ResourceTag())
	jwt, err := jwtGenerator.MakeLoginToken(ctx, user)
	if err != nil {
		return ControllerInfo{}, errors.E(err, "cannot generate jwt")
	}

	return ControllerInfo{
		Addresses: addrs,
		JWT:       string(jwt),
	}, nil
}
