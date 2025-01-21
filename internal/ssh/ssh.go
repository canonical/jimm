// Copyright 2025 Canonical.

package ssh

import (
	"context"
	"fmt"
	"io"
	"net"
	"time"

	"github.com/gliderlabs/ssh"
	"github.com/juju/names/v5"
	"github.com/juju/zaputil/zapctx"
	"go.uber.org/zap"
	gossh "golang.org/x/crypto/ssh"

	"github.com/canonical/jimm/v3/internal/openfga"
)

// jujuSSHDefaultPort is the default port we expect the juju controllers to respond on.
const jujuSSHDefaultPort = 17022
const defaultAcceptConnectionTimeout = time.Second
const defaultMaxConcurrentConnections = 100

type publicKeySSHUserKey struct{}

// SSHAuthorizer is the interface to authorize users via public key.
type SSHAuthorizer interface {
	// PublicKeyHandler is the method to verify the public key of the user. It returns a user if successful.
	PublicKeyHandler(ctx context.Context, claimUser string, key []byte) (*openfga.User, error)
}

// TODO(simonedutto): this is going to change to reuse as much as our dial logic as we possibly can.
// SSHResolver is the interface to resolve controller's addresses.
type SSHResolver interface {
	// AddrFromModelUUID is the method to resolve the address of the controller to contact given the model UUID.
	AddrFromModelUUID(ctx context.Context, user *openfga.User, modelTag names.ModelTag) (string, error)
}

// forwardMessage is the struct holding the information about the jump message received by the ssh client.
type forwardMessage struct {
	DestAddr string
	DestPort uint32
	SrcAddr  string
	SrcPort  uint32
}

// Config is the struct holding the configuration for the jump server.
type Config struct {
	Port                     string
	HostKey                  []byte
	MaxConcurrentConnections int
	AcceptConnectionTimeout  time.Duration
}

// Server is the struct holding the jump server and some
type Server struct {
	*ssh.Server

	maxConcurrentConnections int
	acceptConnectionTimeout  time.Duration
}

// NewJumpServer creates the jump server struct.
func NewJumpServer(ctx context.Context, config Config, sshAuthorizer SSHAuthorizer, sshResolver SSHResolver) (Server, error) {
	zapctx.Info(ctx, "NewJumpServer")

	if sshResolver == nil {
		return Server{}, fmt.Errorf("Cannot create JumpSSHServer with a nil resolver.")
	}
	config = setConfigDefaults(config)
	server := Server{
		Server: &ssh.Server{
			Addr: fmt.Sprintf(":%s", config.Port),
			ChannelHandlers: map[string]ssh.ChannelHandler{
				"direct-tcpip": directTCPIPHandler(sshResolver),
			},
			PublicKeyHandler: func(ctx ssh.Context, key ssh.PublicKey) bool {
				user, err := sshAuthorizer.PublicKeyHandler(ctx, ctx.User(), key.Marshal())
				if err != nil {
					zapctx.Debug(ctx, fmt.Sprintf("cannot verify key for user %s", ctx.User()), zap.Error(err))
					return false
				}
				ctx.SetValue(publicKeySSHUserKey{}, user)
				return true
			},
		},
		maxConcurrentConnections: config.MaxConcurrentConnections,
		acceptConnectionTimeout:  config.AcceptConnectionTimeout,
	}
	hostKey, err := gossh.ParsePrivateKey([]byte(config.HostKey))
	if err != nil {
		return Server{}, fmt.Errorf("Cannot parse hostkey.")
	}
	server.AddHostKey(hostKey)

	return server, nil
}

// setConfigDefaults sets the default values for the configuration.
func setConfigDefaults(config Config) Config {
	if config.Port == "" {
		config.Port = fmt.Sprint(jujuSSHDefaultPort)
	}
	if config.MaxConcurrentConnections <= 0 {
		config.MaxConcurrentConnections = defaultMaxConcurrentConnections
	}
	if config.AcceptConnectionTimeout <= 0 {
		config.AcceptConnectionTimeout = defaultAcceptConnectionTimeout
	}
	return config
}

// ListenAndServe create a LimitListenerWithTimeout and Serve requests.
func (srv Server) ListenAndServe() error {
	ln, err := net.Listen("tcp", srv.Addr)
	ln = limitListenerWithTimeout(ln, srv.maxConcurrentConnections, srv.acceptConnectionTimeout)
	if err != nil {
		return err
	}
	return srv.Serve(ln)
}

func directTCPIPHandler(sshResolver SSHResolver) func(srv *ssh.Server, conn *gossh.ServerConn, newChan gossh.NewChannel, ctx ssh.Context) {
	return func(srv *ssh.Server, conn *gossh.ServerConn, newChan gossh.NewChannel, ctx ssh.Context) {
		d := forwardMessage{}
		k := newChan.ExtraData()

		if err := gossh.Unmarshal(k, &d); err != nil {
			rejectConnectionAndLogError(ctx, newChan, "failed to parse channel data", err)
			return
		}
		if d.DestPort == 0 {
			d.DestPort = jujuSSHDefaultPort
		}
		if !names.IsValidModel(d.DestAddr) {
			rejectConnectionAndLogError(ctx, newChan, "invalid model uuid", nil)
			return
		}
		modelTag := names.NewModelTag(d.DestAddr)
		user, err := fetchAndAuthorizeUser(ctx, modelTag)
		if err != nil {
			rejectConnectionAndLogError(ctx, newChan, err.Error(), err)
			return
		}
		addr, err := sshResolver.AddrFromModelUUID(ctx, user, modelTag)
		if err != nil {
			rejectConnectionAndLogError(ctx, newChan, "failed to resolve address from model uuid", err)
			return
		}
		dest := net.JoinHostPort(addr, fmt.Sprint(d.DestPort))
		// this is temporary. The way we dial to the controller will heavily change.
		client, err := gossh.Dial("tcp", dest, &gossh.ClientConfig{
			//nolint:gosec // this will be removed once we handle hostkeys
			HostKeyCallback: gossh.InsecureIgnoreHostKey(),
			Auth: []gossh.AuthMethod{
				gossh.PasswordCallback(func() (secret string, err error) {
					return "jwt", nil
				}),
			},
		})
		if err != nil {
			rejectConnectionAndLogError(ctx, newChan, fmt.Sprintf("failed to connect to %s: %v", dest, err), err)
			return
		}

		dstChan, reqs, err := client.OpenChannel("direct-tcpip", gossh.Marshal(d))
		if err != nil {
			rejectConnectionAndLogError(ctx, newChan, "failed to open destination channel", err)
			return
		}
		// gossh.Request are requests sent outside of the normal stream of data (ex. pty-req for an interactive session).
		// Since we only need the raw data to redirect, we can discard them.
		go gossh.DiscardRequests(reqs)

		srcDest, reqs, err := newChan.Accept()
		if err != nil {
			dstChan.Close()
			return
		}
		// gossh.Request are requests sent outside of the normal stream of data (ex. pty-req for an interactive session).
		// Since we only need the raw data to redirect, we can discard them.
		go gossh.DiscardRequests(reqs)

		go func() {
			defer srcDest.Close()
			defer dstChan.Close()
			_, err := io.Copy(srcDest, dstChan)
			if err != nil {
				rejectConnectionAndLogError(ctx, newChan, "failed to copy data from src to dts", err)
			}
		}()
		go func() {
			defer srcDest.Close()
			defer dstChan.Close()
			_, err := io.Copy(dstChan, srcDest)
			if err != nil {
				rejectConnectionAndLogError(ctx, newChan, "failed to copy data from dst to src", err)
			}
		}()
		zapctx.Info(ctx, fmt.Sprintf("Proxying connection from %s:%d to %s:%d \n", d.SrcAddr, d.SrcPort, d.DestAddr, d.DestPort))
	}
}

// fetchAndAuthorizeUser extracts the user from the context and checks the user has permission to ssh.
func fetchAndAuthorizeUser(ctx ssh.Context, modelTag names.ModelTag) (*openfga.User, error) {
	user, ok := ctx.Value(publicKeySSHUserKey{}).(*openfga.User)
	if !ok {
		return nil, fmt.Errorf("fo user in the context")
	}
	ok, err := user.IsModelWriter(ctx, modelTag)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve address from model uuid")
	}
	if !ok {
		return nil, fmt.Errorf("user doesn't have permission")
	}
	return user, nil
}

// rejectConnectionAndLogError logs the error and rejects the channel with a message.
func rejectConnectionAndLogError(ctx context.Context, newChan gossh.NewChannel, msg string, err error) {
	zapctx.Error(ctx, msg, zap.Error(err))
	err = newChan.Reject(gossh.ConnectionFailed, msg)
	if err != nil {
		zapctx.Error(ctx, msg, zap.Error(err))
	}
}
