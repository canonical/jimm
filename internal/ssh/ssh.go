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

	jimmssh "github.com/canonical/jimm/v3/internal/jimm/ssh"
	"github.com/canonical/jimm/v3/internal/openfga"
	"github.com/canonical/jimm/v3/internal/ssh/limitlistener"
)

const defaultSSHPort = 17022
const defaultAcceptConnectionTimeout = time.Second
const defaultMaxConcurrentConnections = 100

type publicKeySSHUserKey struct{}

// SSHManager is the interface to enable the ssh server to operate. Performing public key verification and
// resolving addresses from model uuids.
type SSHManager interface {
	// PublicKeyHandler is the method to verify the public key of the user. It returns a user if successful.
	PublicKeyHandler(ctx context.Context, claimUser string, key []byte) (*openfga.User, error)

	// ControllerInfoFromModelUUID uses the given model UUID to return the address of the controller to
	// contact and a valid JWT To authenticate to the controller.
	ControllerInfoFromModelUUID(ctx context.Context, modelUUID string, user *openfga.User) (jimmssh.ControllerInfo, error)

	// DialControllerSSHServer dials the controller using the provided controller info.
	DialControllerSSHServer(ctx context.Context, ctrlInfo jimmssh.ControllerInfo, user *openfga.User) (*gossh.Client, error)
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
func NewJumpServer(ctx context.Context, config Config, sshManager SSHManager) (Server, error) {
	zapctx.Info(ctx, "NewJumpServer")

	if sshManager == nil {
		return Server{}, fmt.Errorf("Cannot create JumpSSHServer with a nil ssh manager.")
	}
	config = setConfigDefaults(config)

	server := Server{
		Server: &ssh.Server{
			Addr: fmt.Sprintf(":%s", config.Port),
			ChannelHandlers: map[string]ssh.ChannelHandler{
				"direct-tcpip": directTCPIPHandler(sshManager),
			},
			PublicKeyHandler: func(ctx ssh.Context, key ssh.PublicKey) bool {
				user, err := sshManager.PublicKeyHandler(ctx, ctx.User(), key.Marshal())
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
		config.Port = fmt.Sprint(defaultSSHPort)
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
	ln = limitlistener.ListenerWithTimeout(ln, srv.maxConcurrentConnections, srv.acceptConnectionTimeout)
	if err != nil {
		return err
	}
	return srv.Serve(ln)
}

func directTCPIPHandler(sshManager SSHManager) func(srv *ssh.Server, conn *gossh.ServerConn, newChan gossh.NewChannel, ctx ssh.Context) {
	return func(srv *ssh.Server, conn *gossh.ServerConn, newChan gossh.NewChannel, ctx ssh.Context) {
		d := forwardMessage{}
		k := newChan.ExtraData()

		if err := gossh.Unmarshal(k, &d); err != nil {
			rejectConnectionAndLogError(ctx, newChan, "failed to parse channel data", err)
			return
		}

		// TODO: Parse destAddr from a virtual hostname.

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

		connInfo, err := sshManager.ControllerInfoFromModelUUID(ctx, modelTag.Id(), user)
		if err != nil {
			rejectConnectionAndLogError(ctx, newChan, "failed to get controller connection info", err)
			return
		}

		client, err := sshManager.DialControllerSSHServer(ctx, connInfo, user)
		if err != nil {
			rejectConnectionAndLogError(ctx, newChan, "failed to dial controller", err)
			return
		}

		controllerConn, err := client.Dial("tcp", fmt.Sprintf("%s:22", d.DestAddr))
		if err != nil {
			rejectConnectionAndLogError(ctx, newChan, "failed to create tunnel to controller", err)
			return
		}

		clientConn, reqs, err := newChan.Accept()
		if err != nil {
			rejectConnectionAndLogError(ctx, newChan, "failed to accept channel creation request", err)
			return
		}
		// gossh.Request are requests sent outside of the normal stream of data (ex. pty-req for an interactive session).
		// Since we only need the raw data to redirect, we can discard them.
		go gossh.DiscardRequests(reqs)

		go func() {
			defer clientConn.Close()
			defer controllerConn.Close()
			_, err = io.Copy(clientConn, controllerConn)
			if err != nil {
				zapctx.Error(ctx, "ssh client to controller error", zap.Error(err))
			}

		}()
		go func() {
			defer clientConn.Close()
			defer controllerConn.Close()
			_, err = io.Copy(controllerConn, clientConn)
			if err != nil {
				zapctx.Error(ctx, "ssh controller to client error", zap.Error(err))
			}
		}()
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
