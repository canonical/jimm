// Copyright 2025 Canonical.

package ssh

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/base64"
	goerr "errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gliderlabs/ssh"
	"github.com/juju/zaputil/zapctx"

	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/jimm/jujuauth"
	"github.com/canonical/jimm/v3/internal/openfga"
	"github.com/canonical/jimm/v3/internal/rpc"
)

// relayUpgradeToken is the custom HTTP upgrade token the controller's
// relay endpoint requires. A bare "ssh" token is not used because
// authentication already happened at the HTTP layer and any SSH client
// could otherwise negotiate the upgrade.
const relayUpgradeToken = "juju-ssh-relay"

// relayDialTimeout bounds each attempt to dial a controller's relay
// endpoint.
const relayDialTimeout = 5 * time.Second

// DialInfo is the struct holding the information
// to dial a controller's SSH relay endpoint.
type DialInfo struct {
	// Addresses to dial the controller's API server, each as host:port.
	Addresses []string

	// TLSConfig authenticates the controller. It must pin ALPN to
	// http/1.1: HTTP/2 disallows the upgrade mechanism.
	TLSConfig *tls.Config

	// JWT is the base64-encoded bearer token authenticating JIMM to the
	// controller.
	JWT string
}

// IdentityManager provides a means to fetch an identity from the identity service.
type IdentityManager interface {
	FetchIdentity(ctx context.Context, id string) (*openfga.User, error)
}

// JujuManager provides a means to fetch a model from the model service.
type JujuManager interface {
	GetModel(ctx context.Context, uuid string) (dbmodel.Model, error)
}

// SSHKeyManager provides a means to manage ssh keys within JIMM.
type SSHKeyManager interface {
	VerifyPublicKey(ctx context.Context, claimUser string, publicKey []byte) (bool, error)
}

// SSHDialer provides a means to establish an upgraded connection to a
// controller's SSH relay endpoint.
type SSHDialer interface {
	// DialRelay dials the controller at addr (host:port of its API server)
	// and performs the HTTP upgrade handshake for the given virtual
	// hostname, returning the raw upgraded connection that carries the
	// user's SSH session bytes.
	DialRelay(ctx context.Context, addr string, tlsConfig *tls.Config, virtualHostname, bearerToken string) (net.Conn, error)
}

// BasicDialer is a wrapper around the default TLS dialer for cases where
// no changes are needed.
type BasicDialer struct{}

// DialRelay implements SSHDialer. It dials the controller's API server
// over TLS, sends the upgrade request, and returns the raw connection
// after a 101 Switching Protocols response.
func (d *BasicDialer) DialRelay(ctx context.Context, addr string, tlsConfig *tls.Config, virtualHostname, bearerToken string) (net.Conn, error) {
	// ALPN must be pinned to http/1.1: HTTP/2 disallows the upgrade
	// mechanism and Hijack returns ErrNotSupported on an h2 connection.
	if tlsConfig == nil {
		tlsConfig = &tls.Config{}
	} else {
		tlsConfig = tlsConfig.Clone()
	}
	tlsConfig.NextProtos = []string{"http/1.1"}

	dialer := &tls.Dialer{
		Config: tlsConfig,
		NetDialer: &net.Dialer{
			Timeout: relayDialTimeout,
		},
	}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("dialing controller %s: %w", addr, err)
	}

	target := &url.URL{
		Scheme: "https",
		Host:   addr,
		Path:  "/ssh-relay/" + virtualHostname,
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("building relay upgrade request: %w", err)
	}
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Upgrade", relayUpgradeToken)
	req.Header.Set("Authorization", "Bearer "+bearerToken)

	if err := req.Write(conn); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("writing relay upgrade request: %w", err)
	}
	// Read the response head with a bounded reader; the connection is
	// handed over raw afterwards so no buffered bytes may be lost.
	resp, err := http.ReadResponse(bufio.NewReader(conn), req)
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("reading relay upgrade response: %w", err)
	}
	if resp.StatusCode != http.StatusSwitchingProtocols {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		_ = resp.Body.Close()
		_ = conn.Close()
		return nil, fmt.Errorf("relay upgrade rejected: %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	_ = resp.Body.Close()
	return conn, nil
}

// SSHManagerParams contains the dependencies
// needed to create the SSHManager service.
type SSHManagerParams struct {
	IdentityManager IdentityManager
	JujuManager     JujuManager
	SSHKeyManager   SSHKeyManager
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
		jwtFactory:      p.JWTFactory,
		dialer:          p.Dialer,
	}, nil
}

// SSHManager provides a means to manage ssh server within JIMM.
type SSHManager struct {
	jujuManager     JujuManager
	identityManager IdentityManager
	sshKeyManager   SSHKeyManager
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
	user, err := s.identityManager.FetchIdentity(ctx, claimUser)
	if err != nil {
		zapctx.Info(ctx, fmt.Sprintf("cannot find user %s", claimUser))
		return nil, fmt.Errorf("cannot find user %s: %v", claimUser, err)
	}
	return user, nil
}

// DialInfo resolves the address of the controller to contact given the
// model UUID and returns a struct with parameters to connect and authenticate
// to the controller's SSH relay endpoint. The context should contain the
// public key the user used to authenticate.
func (s *SSHManager) DialInfo(ctx context.Context, modelUUID string, user *openfga.User) (DialInfo, error) {
	zapctx.Info(ctx, "SSHDialInfo")
	model, err := s.jujuManager.GetModel(ctx, modelUUID)
	if err != nil {
		return DialInfo{}, fmt.Errorf("cannot find model: %v", err)
	}

	addrs, tlsConfig := rpc.GetAddressesAndTLSConfig(ctx, &model.Controller)
	if len(addrs) == 0 {
		return DialInfo{}, errors.New("cannot find addresses for model's controller")
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
		Addresses: addrs,
		TLSConfig: tlsConfig,
		JWT:      base64.StdEncoding.EncodeToString(token),
	}, nil
}

// DialController dials a controller's SSH relay endpoint over an HTTP
// upgrade connection and returns the raw connection carrying the user's
// relayed SSH session bytes. The virtual hostname identifies the
// destination within the controller.
func (s *SSHManager) DialController(ctx context.Context, dialInfo DialInfo, virtualHostname string) (net.Conn, error) {
	var conn net.Conn
	var err error
	var errs []error

	for _, addr := range dialInfo.Addresses {
		conn, err = s.dialer.DialRelay(ctx, addr, dialInfo.TLSConfig, virtualHostname, dialInfo.JWT)
		if err != nil {
			conn = nil
			errs = append(errs, err)
		} else {
			break
		}
	}

	if conn == nil {
		return nil, fmt.Errorf("failed to dial controller: %v", goerr.Join(errs...))
	}
	return conn, nil
}
