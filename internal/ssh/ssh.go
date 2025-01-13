// Copyright 2025 Canonical.

package ssh

import (
	"context"
	"fmt"
	"io"
	"net"

	"github.com/gliderlabs/ssh"
	"github.com/juju/zaputil/zapctx"
	"go.uber.org/zap"
	gossh "golang.org/x/crypto/ssh"

	"github.com/canonical/jimm/v3/internal/openfga"
)

// juju_ssh_default_port is the default port we expect the juju controllers to respond on.
const juju_ssh_default_port = 17022

// Resolver is the interface with the methods needed by the ssh jump server to route request.
type Resolver interface {
	// AddrFromModelUUID is the method to resolve the address of the controller to contact given the model UUID.
	AddrFromModelUUID(ctx context.Context, user openfga.User, modelUUID string) (string, error)
}

// fowardMessage is the struct holding the information about the jump message received by the ssh client.
type forwardMessage struct {
	DestAddr string
	DestPort uint32
	SrcAddr  string
	SrcPort  uint32
}

// Server is the custom struct to embed the gliderlabs.ssh server and a resolver.
type Server struct {
	*ssh.Server

	resolver Resolver
}

// NewJumpSSHServer creates the jump server struct.
func NewJumpSSHServer(ctx context.Context, port int, resolver Resolver) (Server, error) {
	zapctx.Info(ctx, "NewSSHServer")

	if resolver == nil {
		return Server{}, fmt.Errorf("Cannot create JumpSSHServer with a nil resolver.")
	}
	server := Server{
		Server: &ssh.Server{
			Addr: fmt.Sprintf(":%d", port),
			ChannelHandlers: map[string]ssh.ChannelHandler{
				"direct-tcpip": directTCPIPHandler(resolver),
			},
			PublicKeyHandler: func(ctx ssh.Context, key ssh.PublicKey) bool {
				return true
			},
		},
		resolver: resolver,
	}

	return server, nil
}

func directTCPIPHandler(resolver Resolver) func(srv *ssh.Server, conn *gossh.ServerConn, newChan gossh.NewChannel, ctx ssh.Context) {
	return func(srv *ssh.Server, conn *gossh.ServerConn, newChan gossh.NewChannel, ctx ssh.Context) {
		d := forwardMessage{}

		k := newChan.ExtraData()

		if err := gossh.Unmarshal(k, &d); err != nil {
			rejectConnectionAndLogError(ctx, newChan, "Failed to parse channel data", err)
			return
		}
		if d.DestPort == 0 {
			d.DestPort = juju_ssh_default_port
		}
		addr, err := resolver.AddrFromModelUUID(ctx, openfga.User{}, d.DestAddr)
		if err != nil {
			rejectConnectionAndLogError(ctx, newChan, "Failed to resolve address from model uuid", err)
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
			rejectConnectionAndLogError(ctx, newChan, fmt.Sprintf("Failed to connect to %s: %v", dest, err), err)
			return
		}

		dstChan, reqs, err := client.OpenChannel("direct-tcpip", gossh.Marshal(d))
		if err != nil {
			rejectConnectionAndLogError(ctx, newChan, "Failed to open destination channel", err)
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
				rejectConnectionAndLogError(ctx, newChan, "Failed to copy data from src to dts", err)
			}
		}()
		go func() {
			defer srcDest.Close()
			defer dstChan.Close()
			_, err := io.Copy(dstChan, srcDest)
			if err != nil {
				rejectConnectionAndLogError(ctx, newChan, "Failed to copy data from dst to src", err)
			}
		}()
		zapctx.Info(ctx, fmt.Sprintf("Proxying connection from %s:%d to %s:%d \n", d.SrcAddr, d.SrcPort, d.DestAddr, d.DestPort))
	}
}

func rejectConnectionAndLogError(ctx context.Context, newChan gossh.NewChannel, msg string, err error) {
	zapctx.Error(ctx, msg, zap.Error(err))
	err = newChan.Reject(gossh.ConnectionFailed, msg)
	if err != nil {
		zapctx.Error(ctx, msg, zap.Error(err))
	}
}
