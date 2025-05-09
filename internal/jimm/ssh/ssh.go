// Copyright 2025 Canonical.

package ssh

import (
	"context"
	"encoding/base64"
	goerr "errors"
	"fmt"
	"net"
	"time"

	"github.com/gliderlabs/ssh"
	jujucontroller "github.com/juju/juju/controller"
	"github.com/juju/zaputil/zapctx"
	gossh "golang.org/x/crypto/ssh"

	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/jimm/jujuauth"
	"github.com/canonical/jimm/v3/internal/openfga"
	"github.com/canonical/jimm/v3/internal/rpc"
)

// DialInfo is the struct holding the infomation
// to dial a controller via SSH.
type DialInfo struct {
	// addresses to dial the controller
	Addresses []string

	// Port to establish the SSH connection
	Port int

	// JWT to authenticate to the controller
	JWT string
}

// IdentityManager provides a means to fetch an identity from the identity service.
type IdentityManager interface {
	FetchIdentity(ctx context.Context, id string) (*openfga.User, error)
}

// JujuManager provides a means to fetch a model from the model service.
type JujuManager interface {
	GetModel(ctx context.Context, uuid string) (dbmodel.Model, error)
	ControllerConfig(ctx context.Context, controllerName string) (jujucontroller.Config, error)
}

// SSHKeyManager provides a means to manage ssh keys within JIMM.
type SSHKeyManager interface {
	VerifyPublicKey(ctx context.Context, claimUser string, publicKey []byte) (bool, error)
}

// NewSSHManager returns a new SSHManager that offers jimm functionality to the SSHJumpServer.
func NewSSHManager(identityManager IdentityManager, jujuManager JujuManager, sshKeyManager SSHKeyManager, jwtFactory *jujuauth.Factory) (*sshManager, error) {
	if identityManager == nil {
		return nil, errors.E("identityManager cannot be nil")
	}
	if jujuManager == nil {
		return nil, errors.E("jujuManager cannot be nil")
	}
	if sshKeyManager == nil {
		return nil, errors.E("sshManager cannot be nil")
	}
	if jwtFactory == nil {
		return nil, errors.E("jwtFactory cannot be nil")
	}
	return &sshManager{
		jujuManager:     jujuManager,
		identityManager: identityManager,
		sshKeyManager:   sshKeyManager,
		jwtFactory:      jwtFactory,
	}, nil
}

// sshManager provides a means to manage ssh server within JIMM.
type sshManager struct {
	jujuManager     JujuManager
	identityManager IdentityManager
	sshKeyManager   SSHKeyManager
	jwtFactory      *jujuauth.Factory
}

// PublicKeyHandler is the method to verify the public key of the user. It first checks for the public key and then fetches the user.
// It returns a user if successful.
func (s *sshManager) PublicKeyHandler(ctx context.Context, claimUser string, key []byte) (*openfga.User, error) {
	zapctx.Info(ctx, "PublicKeyHandler")
	if ok, err := s.sshKeyManager.VerifyPublicKey(ctx, claimUser, key); !ok || err != nil {
		return nil, fmt.Errorf("cannot verify key for user %s: %v", claimUser, err)
	}
	user, err := s.identityManager.FetchIdentity(ctx, claimUser)
	if err != nil {
		zapctx.Info(ctx, fmt.Sprintf("cannot find user %s", claimUser))
		return nil, fmt.Errorf("cannot find user %s: %v", claimUser, err)
	}
	return user, nil
}

// DialInfo resolves the address of the controller to contact given the
// model UUID and returns a struct with parameters to connect and authenticate
// to the controller. The context should contain the public key the user
// used to authenticate.
func (s *sshManager) DialInfo(ctx context.Context, modelUUID string, user *openfga.User) (DialInfo, error) {
	zapctx.Info(ctx, "SSHDialInfo")
	model, err := s.jujuManager.GetModel(ctx, modelUUID)
	if err != nil {
		return DialInfo{}, fmt.Errorf("cannot find model: %v", err)
	}

	controllerConfig, err := s.jujuManager.ControllerConfig(ctx, model.Controller.Name)
	if err != nil {
		return DialInfo{}, errors.E(err, "cannot get controller config")
	}

	addrs, _ := rpc.GetAddressesAndTLSConfig(ctx, &model.Controller)
	if len(addrs) == 0 {
		return DialInfo{}, fmt.Errorf("cannot find addresses for model's controller: %v", err)
	}

	addrsNoPort := make([]string, len(addrs))
	for i, addr := range addrs {
		hostNoPort, _, err := net.SplitHostPort(addr)
		// If there was an error we will assume there is no port since
		// SplitHostPort doesn't expose const error types for checking.
		if err != nil {
			addrsNoPort[i] = addr
		} else {
			addrsNoPort[i] = hostNoPort
		}
	}

	publicKey, _ := ctx.Value(ssh.ContextKeyPublicKey).(ssh.PublicKey)
	if publicKey == nil {
		return DialInfo{}, errors.E("cannot find user's public key")
	}

	tokenArgs := jujuauth.SSHTokenArgs{
		User:           user.Name,
		ControllerUUID: model.Controller.UUID,
		ModelTag:       model.Tag(),
		PublicKey:      publicKey.Marshal(),
	}
	jwtGenerator := s.jwtFactory.NewSSHGenerator()
	token, err := jwtGenerator.NewSSHToken(ctx, tokenArgs)
	if err != nil {
		return DialInfo{}, fmt.Errorf("cannot generate jwt: %v", err)
	}

	return DialInfo{
		Addresses: addrsNoPort,
		Port:      controllerConfig.SSHServerPort(),
		JWT:       base64.StdEncoding.EncodeToString(token),
	}, nil
}

// DialController dials a controller's SSH
// server and returns an SSH connection.
func (s *sshManager) DialController(ctx context.Context, dialInfo DialInfo, user *openfga.User) (*gossh.Client, error) {
	var client *gossh.Client
	var err error
	var errs []error

	for _, addr := range dialInfo.Addresses {
		dest := net.JoinHostPort(addr, fmt.Sprint(dialInfo.Port))
		client, err = gossh.Dial("tcp", dest, &gossh.ClientConfig{
			User: "jimm",
			//nolint:gosec // this will be removed once we handle hostkeys
			HostKeyCallback: gossh.InsecureIgnoreHostKey(),
			Auth: []gossh.AuthMethod{
				gossh.PasswordCallback(func() (secret string, err error) {
					return dialInfo.JWT, nil
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
