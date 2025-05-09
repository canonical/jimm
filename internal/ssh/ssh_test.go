// Copyright 2025 Canonical.

package ssh_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"testing"
	"time"

	qt "github.com/frankban/quicktest"
	"github.com/frankban/quicktest/qtsuite"
	gliderssh "github.com/gliderlabs/ssh"
	"github.com/juju/juju/core/virtualhostname"
	"github.com/juju/names/v5"
	gossh "golang.org/x/crypto/ssh"
	"google.golang.org/grpc/test/bufconn"

	"github.com/canonical/jimm/v3/internal/db"
	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
	jimmssh "github.com/canonical/jimm/v3/internal/jimm/ssh"
	"github.com/canonical/jimm/v3/internal/openfga"
	ofganames "github.com/canonical/jimm/v3/internal/openfga/names"
	"github.com/canonical/jimm/v3/internal/ssh"
	"github.com/canonical/jimm/v3/internal/testutils/jimmtest"
	"github.com/canonical/jimm/v3/internal/testutils/jimmtest/mocks"
)

type sshSuite struct {
	destinationJujuSSHServer *gliderssh.Server
	testInDestinationServerF func(fm ssh.ForwardMessage)

	jumpSSHServer      ssh.Server
	jumpServerListener *bufconn.Listener
	privateKey         gossh.Signer
	hostKey            gossh.Signer

	received                 chan bool
	virtualHostname          virtualhostname.Info
	maxConcurrentConnections int
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
	s.virtualHostname, err = virtualhostname.Parse("1.postgresql.deadbeef-1bad-500d-9000-4b1d0d06f00d.juju.local")
	c.Assert(err, qt.IsNil)
	err = userWithAccess.SetModelAccess(ctx, names.NewModelTag(s.virtualHostname.ModelUUID()), ofganames.AdministratorRelation)
	c.Assert(err, qt.IsNil)

	// create a user and don't set any permission
	i2, err := dbmodel.NewIdentity("bob")
	c.Assert(err, qt.IsNil)
	userWithoutAccess := openfga.NewUser(i2, ofgaClient)

	// setup destination server
	s.received = make(chan bool)
	destinationServerListener := bufconn.Listen(1 * 1024)

	s.destinationJujuSSHServer = &gliderssh.Server{
		ChannelHandlers: map[string]gliderssh.ChannelHandler{
			"direct-tcpip": func(srv *gliderssh.Server, conn *gossh.ServerConn, newChan gossh.NewChannel, ctx gliderssh.Context) {
				d := ssh.ForwardMessage{}
				if err := gossh.Unmarshal(newChan.ExtraData(), &d); err != nil {
					err := newChan.Reject(gossh.ConnectionFailed, "Failed to parse channel data")
					c.Check(err, qt.IsNil)
					return
				}
				_, _, err := newChan.Accept()
				c.Check(err, qt.IsNil)
				s.testInDestinationServerF(d)
				s.received <- true
			},
		},
		PasswordHandler: func(ctx gliderssh.Context, password string) bool {
			return "valid-jwt" == password
		},
	}
	go func() {
		_ = s.destinationJujuSSHServer.Serve(destinationServerListener)
	}()

	// setup jump server
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

	s.jumpServerListener = bufconn.Listen(1 * 1024)
	s.maxConcurrentConnections = 10
	jumpServer, err := ssh.NewJumpServer(context.Background(),
		ssh.Config{
			Port:                     "22",
			HostKey:                  hostKey,
			MaxConcurrentConnections: s.maxConcurrentConnections,
		},
		mocks.SSHManager{
			PublicKeyHandler_: func(ctx context.Context, claimUser string, key []byte) (*openfga.User, error) {
				if claimUser == "alice" {
					return userWithAccess, nil
				}
				return userWithoutAccess, nil
			},
			DialInfo_: func(ctx context.Context, modelUUID string, user *openfga.User) (jimmssh.DialInfo, error) {
				if modelUUID != s.virtualHostname.ModelUUID() {
					return jimmssh.DialInfo{}, errors.E("permission denied")
				}
				return jimmssh.DialInfo{}, nil
			},
			DialController_: func(ctx context.Context, ctrlInfo jimmssh.DialInfo, user *openfga.User) (*gossh.Client, error) {
				conn, err := destinationServerListener.Dial()
				if err != nil {
					return nil, err
				}
				sshConn, newChan, reqs, err := gossh.NewClientConn(conn, "", &gossh.ClientConfig{
					//nolint:gosec
					HostKeyCallback: gossh.InsecureIgnoreHostKey(),
					Auth: []gossh.AuthMethod{
						gossh.Password("valid-jwt"),
					},
					User: user.Name,
				})
				if err != nil {
					return nil, err
				}

				return gossh.NewClient(sshConn, newChan, reqs), nil
			},
		})
	c.Assert(err, qt.IsNil)

	s.jumpSSHServer = jumpServer
	go func() {
		_ = s.jumpSSHServer.Serve(s.jumpServerListener)
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
		c.Check(destinationServerListener.Close(), qt.IsNil)
		c.Check(s.jumpServerListener.Close(), qt.IsNil)
		c.Check(s.destinationJujuSSHServer.Close(), qt.IsNil)
		c.Check(s.jumpSSHServer.Close(), qt.IsNil)
	})
}

func (s *sshSuite) TestSSHJump(c *qt.C) {
	client := inMemoryDial(c, s.jumpServerListener, &gossh.ClientConfig{
		HostKeyCallback: gossh.FixedHostKey(s.hostKey.PublicKey()),
		Auth: []gossh.AuthMethod{
			gossh.PublicKeys(s.privateKey),
		},
		User: "alice",
	})
	defer client.Close()

	// send forward message
	s.testInDestinationServerF = func(fm ssh.ForwardMessage) {
		c.Check(fm.DestAddr, qt.Equals, s.virtualHostname.String())
	}
	conn, err := client.Dial("tcp", fmt.Sprintf("%s:22", s.virtualHostname))
	c.Assert(err, qt.IsNil)
	defer conn.Close()
	select {
	case <-s.received:
	case <-time.After(100 * time.Millisecond):
		c.Fail()
	}
}

func (s *sshSuite) TestSSHJumpPermissionFail(c *qt.C) {
	modelUUID := "982b16d9-a945-4762-b684-fd4fd885aa11"
	fakeDestination, err := virtualhostname.NewInfoMachineTarget(modelUUID, "0")
	c.Assert(err, qt.IsNil)

	tests := []struct {
		name     string
		user     string
		destAddr string
		errMsg   string
	}{
		{
			name:     "alice not allowed on this model",
			user:     "alice",
			destAddr: fakeDestination.String(),
			errMsg:   "ssh: rejected: connect failed (user doesn't have permission)",
		},
		{
			name:     "bob not allowed on this model",
			user:     "bob",
			destAddr: s.virtualHostname.String(),
			errMsg:   "ssh: rejected: connect failed (user doesn't have permission)",
		},
		{
			name:     "not existing user",
			user:     "mark",
			destAddr: s.virtualHostname.String(),
			errMsg:   "ssh: rejected: connect failed (user doesn't have permission)",
		},
	}

	for _, test := range tests {
		c.Run(test.name, func(c *qt.C) {
			client := inMemoryDial(c, s.jumpServerListener, &gossh.ClientConfig{
				HostKeyCallback: gossh.FixedHostKey(s.hostKey.PublicKey()),
				Auth: []gossh.AuthMethod{
					gossh.PublicKeys(s.privateKey),
				},
				User: test.user,
			})
			defer client.Close()

			_, err := client.Dial("tcp", fmt.Sprintf("%s:22", test.destAddr))
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

func (s *sshSuite) TestInvalidVirtualHostname(c *qt.C) {
	client := inMemoryDial(c, s.jumpServerListener, &gossh.ClientConfig{
		HostKeyCallback: gossh.FixedHostKey(s.hostKey.PublicKey()),
		Auth: []gossh.AuthMethod{
			gossh.PublicKeys(s.privateKey),
		},
		User: "alice",
	})
	defer client.Close()

	s.testInDestinationServerF = func(fm ssh.ForwardMessage) {
		c.Check(fm.DestAddr, qt.Equals, "model1")
	}
	_, err := client.Dial("tcp", fmt.Sprintf("%s:%d", "model1", 1))
	c.Assert(err, qt.ErrorMatches, `ssh: rejected: connect failed \(failed to parse destination hostname\)`)
}

func (s *sshSuite) TestSSHServerMaxConnections(c *qt.C) {
	// the reason we repeat this test 2 times is to make sure that closing the connections on
	// the first iteration completely resets the counter on the ssh server side.
	for range 2 {
		clients := make([]*gossh.Client, 0, s.maxConcurrentConnections)
		config := &gossh.ClientConfig{
			HostKeyCallback: gossh.FixedHostKey(s.hostKey.PublicKey()),
			Auth: []gossh.AuthMethod{
				gossh.PublicKeys(s.privateKey),
			},
			User: "alice",
		}
		for range s.maxConcurrentConnections {
			client := inMemoryDial(c, s.jumpServerListener, config)
			clients = append(clients, client)
		}
		jumpServerConn, err := s.jumpServerListener.Dial()
		c.Assert(err, qt.IsNil)

		_, _, _, err = gossh.NewClientConn(jumpServerConn, "", config)
		c.Assert(err, qt.ErrorMatches, ".*handshake failed: EOF.*")

		// close the connections
		for _, client := range clients {
			client.Close()
		}
		// check the next connection is accepted
		client := inMemoryDial(c, s.jumpServerListener, config)
		client.Close()
	}
}

func TestSSHSuite(t *testing.T) {
	qtsuite.Run(qt.New(t), &sshSuite{})
}

// inMemoryDial returns and SSH connection that uses an in-memory transport.
func inMemoryDial(c *qt.C, listener *bufconn.Listener, config *gossh.ClientConfig) *gossh.Client {
	jumpServerConn, err := listener.Dial()
	c.Assert(err, qt.IsNil)

	sshConn, newChan, reqs, err := gossh.NewClientConn(jumpServerConn, "", config)
	c.Assert(err, qt.IsNil)
	return gossh.NewClient(sshConn, newChan, reqs)
}
