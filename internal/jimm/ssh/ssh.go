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
	"github.com/juju/names/v5"
	"github.com/juju/zaputil/zapctx"
	gossh "golang.org/x/crypto/ssh"

	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/jimm/jujuauth"
	"github.com/canonical/jimm/v3/internal/jimm/offer"
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
	ControllerConfig(ctx context.Context, user *openfga.User, controllerName string) (jujucontroller.Config, error)
}

// SSHKeyManager provides a means to manage ssh keys within JIMM.
type SSHKeyManager interface {
	VerifyPublicKey(ctx context.Context, claimUser string, publicKey []byte) (bool, error)
}

// SSHDialer provides a means to establish an SSH connection.
type SSHDialer interface {
	Dial(network string, addr string, config *gossh.ClientConfig) (*gossh.Client, error)
}

// BasicDialer is a wrapper around the default Go x/crypto/ssh
// dialer for cases where no changes are needed.
type BasicDialer struct{}

func (d *BasicDialer) Dial(network string, addr string, config *gossh.ClientConfig) (*gossh.Client, error) {
	return gossh.Dial(network, addr, config)
}

// SSHManagerParams contains the dependencies
// needed to create the SSHManager service.
type SSHManagerParams struct {
	IdentityManager IdentityManager
	JujuManager     JujuManager
	SSHKeyManager   SSHKeyManager
	IdPGroupFetcher offer.IdPGroupFetcher
	OpenFGAClient   *openfga.OFGAClient
	JWTFactory      *jujuauth.Factory
	Dialer          SSHDialer
}

func (p *SSHManagerParams) validate() error {
	if p.IdentityManager == nil {
		return errors.New("identityManager cannot be nil")
	}
	if p.JujuManager == nil {
		return errors.New("jujuManager cannot be nil")
	}
	if p.SSHKeyManager == nil {
		return errors.New("sshManager cannot be nil")
	}
	if p.IdPGroupFetcher == nil {
		return errors.New("idp group fetcher cannot be nil")
	}
	if p.OpenFGAClient == nil {
		return errors.New("openfga client cannot be nil")
	}
	if p.JWTFactory == nil {
		return errors.New("jwtFactory cannot be nil")
	}
	if p.Dialer == nil {
		return errors.New("dialer cannot be nil")
	}
	return nil
}

// NewSSHManager returns a new SSHManager that offers domain functionality to the SSHJumpServer.
func NewSSHManager(p SSHManagerParams) (*SSHManager, error) {
	if err := p.validate(); err != nil {
		return nil, err
	}
	return &SSHManager{
		jujuManager:     p.JujuManager,
		identityManager: p.IdentityManager,
		sshKeyManager:   p.SSHKeyManager,
		idpGroupFetcher: p.IdPGroupFetcher,
		openfgaClient:   p.OpenFGAClient,
		jwtFactory:      p.JWTFactory,
		dialer:          p.Dialer,
	}, nil
}

// SSHManager provides a means to manage ssh server within JIMM.
type SSHManager struct {
	jujuManager     JujuManager
	identityManager IdentityManager
	sshKeyManager   SSHKeyManager
	idpGroupFetcher offer.IdPGroupFetcher
	openfgaClient   *openfga.OFGAClient
	jwtFactory      *jujuauth.Factory
	dialer          SSHDialer
}

// PublicKeyHandler is the method to verify the public key of the user. It first checks for the public key and then fetches the user.
// It returns a user if successful.
func (s *SSHManager) PublicKeyHandler(ctx context.Context, claimUser string, key []byte) (*openfga.User, error) {
	zapctx.Info(ctx, "PublicKeyHandler")
	if ok, err := s.sshKeyManager.VerifyPublicKey(ctx, claimUser, key); !ok || err != nil {
		return nil, fmt.Errorf("cannot verify key for user %s: %v", claimUser, err)
	}
	if !names.IsValidUser(claimUser) {
		return nil, fmt.Errorf("cannot parse user %s: invalid username", claimUser)
	}
	tag := names.NewUserTag(claimUser)
	user, err := openfga.NewUserFromTag(ctx, tag, s.openfgaClient, s.idpGroupFetcher)
	if err != nil {
		return nil, err
	}
	return user, nil
}

// DialInfo resolves the address of the controller to contact given the
// model UUID and returns a struct with parameters to connect and authenticate
// to the controller. The context should contain the public key the user
// used to authenticate.
func (s *SSHManager) DialInfo(ctx context.Context, modelUUID string, user *openfga.User) (DialInfo, error) {
	zapctx.Info(ctx, "SSHDialInfo")
	model, err := s.jujuManager.GetModel(ctx, modelUUID)
	if err != nil {
		return DialInfo{}, fmt.Errorf("cannot find model: %v", err)
	}

	controllerConfig, err := s.jujuManager.ControllerConfig(ctx, user, model.Controller.Name)
	if err != nil {
		return DialInfo{}, fmt.Errorf("cannot get controller config: %w", err)
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
		return DialInfo{}, errors.New("cannot find user's public key")
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
func (s *SSHManager) DialController(ctx context.Context, dialInfo DialInfo, user *openfga.User) (*gossh.Client, error) {
	var client *gossh.Client
	var err error
	var errs []error

	for _, addr := range dialInfo.Addresses {
		dest := net.JoinHostPort(addr, fmt.Sprint(dialInfo.Port))
		client, err = s.dialer.Dial("tcp", dest, &gossh.ClientConfig{
			User: "external-auth",
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
		} else {
			break
		}
	}

	if client == nil {
		return nil, fmt.Errorf("failed to dial controller: %v", goerr.Join(errs...))
	}
	return client, nil
}
