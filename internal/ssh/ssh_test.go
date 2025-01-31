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
	"github.com/juju/names/v5"
	gossh "golang.org/x/crypto/ssh"

	"github.com/canonical/jimm/v3/internal/db"
	"github.com/canonical/jimm/v3/internal/dbmodel"
	jimmssh "github.com/canonical/jimm/v3/internal/jimm/ssh"
	"github.com/canonical/jimm/v3/internal/openfga"
	ofganames "github.com/canonical/jimm/v3/internal/openfga/names"
	"github.com/canonical/jimm/v3/internal/ssh"
	"github.com/canonical/jimm/v3/internal/testutils/jimmtest"
	"github.com/canonical/jimm/v3/internal/testutils/jimmtest/mocks"
)

type sshSuite struct {
	destinationJujuSSHServer *gliderssh.Server
	destinationServerPort    int
	jumpSSHServer            ssh.Server
	jumpServerPort           int
	privateKey               gossh.Signer
	hostKey                  gossh.Signer
	testInDestinationServerF func(fm ssh.ForwardMessage)
	received                 chan bool

	allowedModelUUID string
}

func (s *sshSuite) Init(c *qt.C) {
	ctx := context.Background()
	// Setup DB
	db := &db.Database{
		DB: jimmtest.PostgresDB(c, time.Now),
	}
	err := db.Migrate(context.Background())
	c.Assert(err, qt.IsNil)

	// Setup OFGA
	ofgaClient, _, _, err := jimmtest.SetupTestOFGAClient(c.Name())
	c.Assert(err, qt.IsNil)

	// create a user and set permission for one model
	i1, err := dbmodel.NewIdentity("alice")
	c.Assert(err, qt.IsNil)
	userWithAccess := openfga.NewUser(i1, ofgaClient)
	s.allowedModelUUID = "deadbeef-1bad-500d-9000-4b1d0d06f00d"
	err = userWithAccess.SetModelAccess(ctx, names.NewModelTag(s.allowedModelUUID), ofganames.WriterRelation)
	c.Assert(err, qt.IsNil)
	// create a user and don't set any permission
	i2, err := dbmodel.NewIdentity("bob")
	c.Assert(err, qt.IsNil)
	userWithoutAccess := openfga.NewUser(i2, ofgaClient)

	// setup destination server
	s.received = make(chan bool)
	port, err := jimmtest.GetFreePort()
	c.Assert(err, qt.IsNil)
	s.destinationServerPort = port
	s.destinationJujuSSHServer = &gliderssh.Server{
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
		PasswordHandler: func(ctx gliderssh.Context, password string) bool {
			return "valid-jwt" == password
		},
	}
	go func() {
		_ = s.destinationJujuSSHServer.ListenAndServe()
	}()
	s.destinationServerPort, err = strconv.Atoi(strings.Split(s.destinationJujuSSHServer.Addr, ":")[1])
	c.Assert(err, qt.IsNil)

	// setup jump server
	port, err = jimmtest.GetFreePort()
	c.Assert(err, qt.IsNil)
	s.jumpServerPort = port
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	c.Assert(err, qt.IsNil)
	hostKey := pem.EncodeToMemory(
		&pem.Block{
			Type:  "RSA PRIVATE KEY",
			Bytes: x509.MarshalPKCS1PrivateKey(k),
		},
	)
	s.hostKey, err = gossh.ParsePrivateKey(hostKey)
	c.Assert(err, qt.IsNil)

	jumpServer, err := ssh.NewJumpServer(context.Background(),
		ssh.Config{
			Port:                     fmt.Sprint(port),
			HostKey:                  hostKey,
			MaxConcurrentConnections: 10,
		},
		mocks.SSHManager{
			PublicKeyHandler_: func(ctx context.Context, claimUser string, key []byte) (*openfga.User, error) {
				if claimUser == "alice" {
					return userWithAccess, nil
				}
				return userWithoutAccess, nil
			},
			ControllerInfoFromModelUUID_: func(ctx context.Context, modelUUID string, user *openfga.User) (jimmssh.ControllerInfo, error) {
				if user == userWithAccess {
					return jimmssh.ControllerInfo{Addresses: []string{""}, JWT: "valid-jwt"}, nil
				}
				return jimmssh.ControllerInfo{Addresses: []string{""}, JWT: ""}, nil
			},
		})
	c.Assert(err, qt.IsNil)
	s.jumpSSHServer = jumpServer
	c.Assert(err, qt.IsNil)
	go func() {
		_ = s.jumpSSHServer.ListenAndServe()
	}()

	// setup private key
	k, err = rsa.GenerateKey(rand.Reader, 2048)
	c.Assert(err, qt.IsNil)
	keyPEM := pem.EncodeToMemory(
		&pem.Block{
			Type:  "RSA PRIVATE KEY",
			Bytes: x509.MarshalPKCS1PrivateKey(k),
		},
	)

	s.privateKey, err = gossh.ParsePrivateKey(keyPEM)
	c.Assert(err, qt.IsNil)

	// cleanup
	c.Cleanup(func() {
		err := s.destinationJujuSSHServer.Close()
		c.Check(err, qt.IsNil)
		err = s.jumpSSHServer.Close()
		c.Check(err, qt.IsNil)
	})
}

func (s *sshSuite) TestSSHJump(c *qt.C) {
	client, err := gossh.Dial("tcp", fmt.Sprintf(":%d", s.jumpServerPort), &gossh.ClientConfig{
		HostKeyCallback: gossh.FixedHostKey(s.hostKey.PublicKey()),
		Auth: []gossh.AuthMethod{
			gossh.PublicKeys(s.privateKey),
		},
		User: "alice",
	})
	c.Assert(err, qt.IsNil)
	defer client.Close()

	// send forward message
	s.testInDestinationServerF = func(fm ssh.ForwardMessage) {
		c.Check(fm.DestAddr, qt.Equals, s.allowedModelUUID)
	}
	conn, err := client.Dial("tcp", fmt.Sprintf("%s:%d", s.allowedModelUUID, s.destinationServerPort))
	c.Check(err, qt.IsNil)
	defer conn.Close()
	select {
	case <-s.received:
	case <-time.After(100 * time.Millisecond):
		c.Fail()
	}
}

func (s *sshSuite) TestSSHJumpPermissionFail(c *qt.C) {
	tests := []struct {
		name     string
		user     string
		destAddr string
		errMsg   string
	}{
		{
			name:     "alice not allowed on this model",
			user:     "alice",
			destAddr: "982b16d9-a945-4762-b684-fd4fd885aa11",
			errMsg:   "ssh: rejected: connect failed (user doesn't have permission)",
		},
		{
			name:     "bob not allowed on this model",
			user:     "bob",
			destAddr: s.allowedModelUUID,
			errMsg:   "ssh: rejected: connect failed (user doesn't have permission)",
		},
		{
			name:     "not existing user",
			user:     "mark",
			destAddr: s.allowedModelUUID,
			errMsg:   "ssh: rejected: connect failed (user doesn't have permission)",
		},
	}

	for _, test := range tests {
		c.Run(test.name, func(c *qt.C) {
			client, err := gossh.Dial("tcp", fmt.Sprintf(":%d", s.jumpServerPort), &gossh.ClientConfig{
				HostKeyCallback: gossh.FixedHostKey(s.hostKey.PublicKey()),
				Auth: []gossh.AuthMethod{
					gossh.PublicKeys(s.privateKey),
				},
				User: test.user,
			})
			c.Assert(err, qt.IsNil)
			defer client.Close()

			_, err = client.Dial("tcp", fmt.Sprintf("%s:%d", test.destAddr, s.destinationServerPort))
			c.Assert(err.Error(), qt.Equals, test.errMsg)
		})
	}
}

func (s *sshSuite) TestSSHJumpDialFail(c *qt.C) {
	_, err := gossh.Dial("tcp", fmt.Sprintf(":%d", 1), &gossh.ClientConfig{
		HostKeyCallback: gossh.FixedHostKey(s.hostKey.PublicKey()),
		Auth: []gossh.AuthMethod{
			gossh.PublicKeys(s.privateKey),
		},
		User: "alice",
	})
	c.Assert(err, qt.ErrorMatches, ".*connect: connection refused.*")
}

func (s *sshSuite) TestSSHFinalDestinationDialFail(c *qt.C) {
	client, err := gossh.Dial("tcp", fmt.Sprintf(":%d", s.jumpServerPort), &gossh.ClientConfig{
		HostKeyCallback: gossh.FixedHostKey(s.hostKey.PublicKey()),
		Auth: []gossh.AuthMethod{
			gossh.PublicKeys(s.privateKey),
		},
		User: "alice",
	})
	c.Assert(err, qt.IsNil)
	s.testInDestinationServerF = func(fm ssh.ForwardMessage) {
		c.Check(fm.DestAddr, qt.Equals, "model1")
	}
	_, err = client.Dial("tcp", fmt.Sprintf("%s:%d", "model1", 1))
	c.Assert(err, qt.ErrorMatches, ".*connect failed.*")
}

func (s *sshSuite) TestMaxConcurrentConnections(c *qt.C) {
	// fill the max of concurrent connection
	maxConcurrentConnections := 10
	clients := make([]*gossh.Client, 0)
	for range maxConcurrentConnections {
		client, err := gossh.Dial("tcp", fmt.Sprintf(":%d", s.jumpServerPort), &gossh.ClientConfig{
			HostKeyCallback: gossh.FixedHostKey(s.hostKey.PublicKey()),
			Auth: []gossh.AuthMethod{
				gossh.PublicKeys(s.privateKey),
			},
			User: "alice",
		})
		c.Check(err, qt.IsNil)
		clients = append(clients, client)
	}
	// this connection is dropped when we are at maximum connections
	_, err := gossh.Dial("tcp", fmt.Sprintf(":%d", s.jumpServerPort), &gossh.ClientConfig{
		HostKeyCallback: gossh.FixedHostKey(s.hostKey.PublicKey()),
		Auth: []gossh.AuthMethod{
			gossh.PublicKeys(s.privateKey),
		},
		User:    "alice",
		Timeout: 50 * time.Millisecond,
	})
	c.Check(err, qt.ErrorMatches, ".*connection reset.*")
	for _, client := range clients {
		client.Close()
	}
}

func TestIdentityManager(t *testing.T) {
	qtsuite.Run(qt.New(t), &sshSuite{})
}
