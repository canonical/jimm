// Copyright 2025 Canonical.

package ssh_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	qt "github.com/frankban/quicktest"
	"github.com/frankban/quicktest/qtsuite"
	gliderssh "github.com/gliderlabs/ssh"
	gossh "golang.org/x/crypto/ssh"

	"github.com/canonical/jimm/v3/internal/openfga"
	"github.com/canonical/jimm/v3/internal/ssh"
	"github.com/canonical/jimm/v3/internal/testutils/jimmtest"
)

type resolver struct{}

func (r resolver) AddrFromModelUUID(ctx context.Context, user openfga.User, modelName string) (string, error) {
	return "", nil
}

type sshSuite struct {
	destinationJujuSSHServer gliderssh.Server
	destinationServerPort    int
	jumpSSHServer            ssh.Server
	jumpServerPort           int
	privateKey               gossh.Signer
	testInDestinationServerF func(fm ssh.ForwardMessage)
	received                 chan bool
}

func (s *sshSuite) Init(c *qt.C) {
	s.received = make(chan bool)
	port, err := jimmtest.GetFreePort()
	c.Assert(err, qt.IsNil)
	s.destinationServerPort = port
	s.destinationJujuSSHServer = gliderssh.Server{
		Addr: fmt.Sprintf(":%d", port),
		ChannelHandlers: map[string]gliderssh.ChannelHandler{
			"direct-tcpip": func(srv *gliderssh.Server, conn *gossh.ServerConn, newChan gossh.NewChannel, ctx gliderssh.Context) {
				d := ssh.ForwardMessage{}
				if err := gossh.Unmarshal(newChan.ExtraData(), &d); err != nil {
					err := newChan.Reject(gossh.ConnectionFailed, "Failed to parse channel data")
					c.Assert(err, qt.IsNil)
					return
				}
				_, _, err := newChan.Accept()
				c.Assert(err, qt.IsNil)
				s.testInDestinationServerF(d)
				s.received <- true
			},
		},
	}
	go func() {
		_ = s.destinationJujuSSHServer.ListenAndServe()
	}()
	s.destinationServerPort, err = strconv.Atoi(strings.Split(s.destinationJujuSSHServer.Addr, ":")[1])
	c.Assert(err, qt.IsNil)

	port, err = jimmtest.GetFreePort()
	c.Assert(err, qt.IsNil)
	s.jumpServerPort = port
	s.jumpSSHServer, err = ssh.NewJumpSSHServer(context.Background(), port, resolver{})
	c.Assert(err, qt.IsNil)
	go func() {
		_ = s.jumpSSHServer.ListenAndServe()
	}()

	k, err := rsa.GenerateKey(rand.Reader, 2048)
	c.Assert(err, qt.IsNil)
	keyPEM := pem.EncodeToMemory(
		&pem.Block{
			Type:  "RSA PRIVATE KEY",
			Bytes: x509.MarshalPKCS1PrivateKey(k),
		},
	)

	s.privateKey, err = gossh.ParsePrivateKey(keyPEM)
	c.Assert(err, qt.IsNil)
	c.Cleanup(func() {
		err := s.destinationJujuSSHServer.Close()
		c.Check(err, qt.IsNil)
		err = s.jumpSSHServer.Close()
		c.Check(err, qt.IsNil)
	})
}

func (s *sshSuite) TestSSHJump(c *qt.C) {
	client, err := gossh.Dial("tcp", fmt.Sprintf(":%d", s.jumpServerPort), &gossh.ClientConfig{
		//nolint:gosec // this will be removed once we handle hostkeys
		HostKeyCallback: gossh.InsecureIgnoreHostKey(),
		Auth: []gossh.AuthMethod{
			gossh.PublicKeys(s.privateKey),
		},
	})
	c.Assert(err, qt.IsNil)
	defer client.Close()

	// send forward message
	msg := ssh.ForwardMessage{
		DestAddr: "model1",
		//nolint:gosec
		DestPort: uint32(s.destinationServerPort),
		SrcAddr:  "localhost",
		SrcPort:  0,
	}
	s.testInDestinationServerF = func(fm ssh.ForwardMessage) {
		c.Check(fm.DestAddr, qt.Equals, "model1")
	}
	ch, _, err := client.OpenChannel("direct-tcpip", gossh.Marshal(&msg))
	c.Check(err, qt.IsNil)
	defer ch.Close()
	select {
	case <-s.received:
	case <-time.After(100 * time.Millisecond):
		c.Fail()
	}
}

func (s *sshSuite) TestSSHJumpDialFail(c *qt.C) {
	_, err := gossh.Dial("tcp", fmt.Sprintf(":%d", 1), &gossh.ClientConfig{
		//nolint:gosec // this will be removed once we handle hostkeys
		HostKeyCallback: gossh.InsecureIgnoreHostKey(),
		Auth: []gossh.AuthMethod{
			gossh.PublicKeys(s.privateKey),
		},
	})
	c.Assert(err, qt.ErrorMatches, ".*connect: connection refused.*")
}

func (s *sshSuite) TestSSHFinalDestinationDialFail(c *qt.C) {

	client, err := gossh.Dial("tcp", fmt.Sprintf(":%d", s.jumpServerPort), &gossh.ClientConfig{
		//nolint:gosec // this will be removed once we handle hostkeys
		HostKeyCallback: gossh.InsecureIgnoreHostKey(),
		Auth: []gossh.AuthMethod{
			gossh.PublicKeys(s.privateKey),
		},
	})
	c.Assert(err, qt.IsNil)

	// send forward message
	msg := ssh.ForwardMessage{
		DestAddr: "model1",
		//nolint:gosec
		DestPort: 1, // the test fails because there is no ssh server on this port.
		SrcAddr:  "localhost",
		SrcPort:  0,
	}
	s.testInDestinationServerF = func(fm ssh.ForwardMessage) {
		c.Check(fm.DestAddr, qt.Equals, "model1")
	}
	_, _, err = client.OpenChannel("direct-tcpip", gossh.Marshal(&msg))
	c.Assert(err, qt.ErrorMatches, ".*connect failed.*")

}

func TestIdentityManager(t *testing.T) {
	qtsuite.Run(qt.New(t), &sshSuite{})
}
