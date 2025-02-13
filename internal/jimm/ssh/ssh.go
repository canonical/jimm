// Copyright 2025 Canonical.

package ssh

import (
	"context"
	goerr "errors"
	"fmt"
	"net"
	"time"

	"github.com/juju/zaputil/zapctx"
	gossh "golang.org/x/crypto/ssh"

	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/jimm/jujuauth"
	"github.com/canonical/jimm/v3/internal/openfga"
	"github.com/canonical/jimm/v3/internal/rpc"
)

// jujuSSHDefaultPort is the default port we expect the juju controllers to respond on.
const jujuSSHDefaultPort = 17022

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

// DialControllerSSHServer dials the controller and returns
// an SSH connection.
func (s *sshManager) DialControllerSSHServer(ctx context.Context, ctrlInfo ControllerInfo, user *openfga.User) (*gossh.Client, error) {
	// TODO: Dial the controller and request it's SSH port
	// here or save it when we add a controller to JIMM.
	destPort := jujuSSHDefaultPort
	var client *gossh.Client
	var err error
	var errs []error

	for _, addr := range ctrlInfo.Addresses {
		dest := net.JoinHostPort(addr, fmt.Sprint(destPort))
		client, err = gossh.Dial("tcp", dest, &gossh.ClientConfig{
			User: "jimm",
			//nolint:gosec // this will be removed once we handle hostkeys
			HostKeyCallback: gossh.InsecureIgnoreHostKey(),
			Auth: []gossh.AuthMethod{
				gossh.PasswordCallback(func() (secret string, err error) {
					return ctrlInfo.JWT, nil
				}),
			},
			Timeout: 5 * time.Second,
		})
		if err != nil {
			errs = append(errs, err)
		}
	}

	if client == nil {
		return nil, errors.E(goerr.Join(errs...), "cannot dial controller")
	}
	return client, nil
}
