// Copyright 2025 Canonical.

package ssh_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"database/sql"
	"encoding/base64"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	qt "github.com/frankban/quicktest"
	"github.com/frankban/quicktest/qtsuite"
	"github.com/juju/names/v6"
	gossh "golang.org/x/crypto/ssh"

	"github.com/canonical/jimm/v3/internal/db"
	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/jimm/identity"
	"github.com/canonical/jimm/v3/internal/jimm/jujuauth"
	"github.com/canonical/jimm/v3/internal/jimm/permissions"
	"github.com/canonical/jimm/v3/internal/jimm/ssh"
	"github.com/canonical/jimm/v3/internal/jimm/sshkeys"
	"github.com/canonical/jimm/v3/internal/jimmjwx"
	"github.com/canonical/jimm/v3/internal/openfga"
	"github.com/canonical/jimm/v3/internal/testutils/jimmtest"
	"github.com/canonical/jimm/v3/internal/testutils/jimmtest/mocks"
	"github.com/canonical/jimm/v3/internal/testutils/testdb"
)

type sshManagerSuite struct {
	publicKey        sshkeys.PublicKey
	allowedModelUUID string
	database         *db.Database
	mockDialer       *mockDialer

	sshManager *ssh.SSHManager

	userWithAccess    *openfga.User
	userWithoutAccess *openfga.User
}

const testSSHManagerEnv = `
cloud-credentials:
- name: test-cred
  cloud: test
  owner: alice@canonical.com
  type: empty
clouds:
- name: test
  type: test
  regions:
  - name: test-region
controllers:
- name: test
  uuid: 00000001-0000-0000-0000-000000000001
  cloud: test
  region: test-region
  public-address: localhost:1234

models:
- name: test-1
  uuid: 00000002-0000-0000-0000-000000000001
  owner: alice@canonical.com
  cloud: test
  region: test-region
  cloud-credential: test-cred
  controller: test
  users:
  - user: alice@canonical.com
    access: admin
users:
- username: alice@canonical.com
  controller-access: superuser
`

func (s *sshManagerSuite) Init(c *qt.C) {
	ctx := context.Background()
	uuid := "00000002-0000-0000-0000-000000000001"
	jimmTag := names.NewControllerTag(uuid)
	// Setup DB

	s.database = &db.Database{
		DB: testdb.PostgresDB(c, time.Now),
	}
	err := s.database.Migrate(context.Background())
	c.Assert(err, qt.IsNil)
	// Setup OFGA
	ofgaClient, _, _, err := jimmtest.SetupTestOFGAClient(c.Name())
	c.Assert(err, qt.IsNil)

	identityManager, err := identity.NewIdentityManager(s.database, ofgaClient)
	c.Assert(err, qt.IsNil)

	// this is a mock non-mock model manager, bandaid until we have a real model manager to avoid creating a whole jimm.
	modelManager := mocks.ModelManager{
		GetModel_: func(ctx context.Context, uuid string) (dbmodel.Model, error) {
			m := dbmodel.Model{
				UUID: sql.NullString{
					String: uuid,
					Valid:  true,
				},
			}
			err := s.database.GetModel(ctx, &m)
			return m, err
		},
	}
	jujuManager := mocks.JujuManager{
		ModelManager: modelManager,
	}
	permissionManager, err := permissions.NewManager(s.database, ofgaClient, uuid, jimmTag)
	c.Assert(err, qt.IsNil)
	jwtFactory := jujuauth.NewFactory(s.database, mocks.JWTService{
		NewJWT_: func(ctx context.Context, j jimmjwx.JWTParams) ([]byte, error) {
			return []byte("jwt"), nil
		},
	}, permissionManager)

	sshKeyManager, err := sshkeys.NewSSHKeyManager(s.database)
	c.Assert(err, qt.IsNil)

	s.mockDialer = &mockDialer{}

	params := ssh.SSHManagerParams{
		IdentityManager: identityManager,
		JujuManager:     &jujuManager,
		SSHKeyManager:   sshKeyManager,
		JWTFactory:      jwtFactory,
		Dialer:          s.mockDialer,
	}
	s.sshManager, err = ssh.NewSSHManager(params)
	c.Assert(err, qt.IsNil)
	env := jimmtest.ParseEnvironment(c, testSSHManagerEnv)
	env.PopulateDB(c, s.database)
	env.PopulateDBAndPermissions(c, jimmTag, s.database, ofgaClient)
	// create a user and set permission for one model
	s.userWithAccess, err = identityManager.FetchIdentity(ctx, env.Users[0].Username)
	c.Assert(err, qt.IsNil)
	s.allowedModelUUID = env.Models[0].UUID

	// create a user without access
	i2, err := dbmodel.NewIdentity("bob")
	c.Assert(err, qt.IsNil)
	c.Assert(s.database.DB.Create(i2).Error, qt.IsNil)
	s.userWithoutAccess = openfga.NewUser(i2, ofgaClient)
	// setup public key
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	c.Assert(err, qt.IsNil)

	pubKey, err := gossh.NewPublicKey(&key.PublicKey)
	c.Assert(err, qt.IsNil)
	s.publicKey = sshkeys.PublicKey{PublicKey: pubKey, Comment: "myComment"}

	c.Assert(err, qt.IsNil)
	err = sshKeyManager.AddUserPublicKey(ctx, s.userWithAccess, db.SSHKeyModelFilter{ModelUUID: s.allowedModelUUID}, s.publicKey)
	c.Assert(err, qt.IsNil)
}

func (s *sshManagerSuite) TestPublicKeyHandler(c *qt.C) {
	ctx := context.Background()

	// Test that the PublicKeyHandler returns the correct user when the public key is valid.
	user, err := s.sshManager.PublicKeyHandler(ctx, s.userWithAccess.Name, s.publicKey.Marshal())
	c.Assert(err, qt.IsNil)
	c.Assert(user.Name, qt.Equals, "alice@canonical.com")

	// Test that the PublicKeyHandler returns an error when the public key is invalid.
	_, err = s.sshManager.PublicKeyHandler(ctx, s.userWithoutAccess.Name, s.publicKey.Marshal())
	c.Assert(err, qt.ErrorMatches, `cannot verify key for user bob: cannot find a matching key for this user`)
}

func (s *sshManagerSuite) TestDialInfo(c *qt.C) {
	ctx := context.Background()

	ctrl := dbmodel.Controller{Name: "test"}
	err := s.database.GetController(ctx, &ctrl)
	c.Assert(err, qt.IsNil)
	c.Assert(ctrl.PublicAddress, qt.Equals, "localhost:1234")

	// Test that the DialInfo returns the correct controller address and user when the model UUID is valid.
	connInfo, err := s.sshManager.DialInfo(ctx, s.allowedModelUUID, s.userWithAccess)
	c.Assert(err, qt.IsNil)
	c.Assert(connInfo.Addresses, qt.HasLen, 1)
	c.Assert(connInfo.Addresses[0], qt.Equals, "localhost:1234")
	c.Assert(connInfo.JWT, qt.Not(qt.HasLen), 0)
	_, err = base64.StdEncoding.DecodeString(connInfo.JWT)
	c.Assert(err, qt.IsNil)

	// Test that the ControllerInfoFromModelUUID returns an error when the model UUID is invalid.
	_, err = s.sshManager.DialInfo(ctx, "not-valid", s.userWithAccess)
	c.Assert(err, qt.ErrorMatches, ".*cannot find model.*")
}

type mockDialer struct {
	validAddress string
	callCount    int
}

func (d *mockDialer) DialRelay(ctx context.Context, addr string, tlsConfig *tls.Config, virtualHostname, bearerToken string) (net.Conn, error) {
	d.callCount++
	if addr == d.validAddress {
		return &net.TCPConn{}, nil
	}
	return nil, errors.New("dial error")
}

func (s *sshManagerSuite) TestDialAllAddresses(c *qt.C) {
	ctx := context.Background()

	dialInfo := ssh.DialInfo{
		Addresses: []string{"10.1.2.3:17070", "10.1.2.4:17070"},
		JWT:       "fake-jwt",
	}

	_, err := s.sshManager.DialController(ctx, dialInfo, "1.postgresql.8419cd78-4993-4c3a-928e-c646226beeee.juju.local")
	c.Assert(err, qt.ErrorMatches, "failed to dial controller: dial error\ndial error")
	c.Assert(s.mockDialer.callCount, qt.Equals, 2)

	s.mockDialer.validAddress = "10.1.2.4:17070"
	// Test that DialController works when there are multiple addresses.
	_, err = s.sshManager.DialController(ctx, dialInfo, "1.postgresql.8419cd78-4993-4c3a-928e-c646226beeee.juju.local")
	c.Assert(err, qt.IsNil)
	c.Assert(s.mockDialer.callCount, qt.Equals, 4)
}

// relayTestServer is an httptest TLS server that emulates the controller's
// relay endpoint. It validates the upgrade request, writes a 101 response
// and optionally some banner bytes, then echoes anything the client sends.
type relayTestServer struct {
	server *httptest.Server

	// statusCode is the HTTP status the handler responds with. When it is
	// http.StatusSwitchingProtocols the connection is hijacked, otherwise
	// the body is returned as an error to the dialer.
	statusCode int
	// rejectBody is written as the response body for non-101 responses.
	rejectBody string
	// banner is written to the raw connection immediately after the 101,
	// before the reader on the dialer side reads its response head, to
	// exercise the buffered-bytes preservation path.
	banner string

	// gotPath, gotUpgrade and gotAuth record what the handler observed on
	// the request so the test can assert the wire contract.
	gotPath    string
	gotUpgrade string
	gotAuth    string
}

func newRelayTestServer(t *testing.T) *relayTestServer {
	rts := &relayTestServer{statusCode: http.StatusSwitchingProtocols}
	rts.server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rts.gotPath = r.URL.Path
		rts.gotUpgrade = r.Header.Get("Upgrade")
		rts.gotAuth = r.Header.Get("Authorization")

		if rts.statusCode != http.StatusSwitchingProtocols {
			http.Error(w, rts.rejectBody, rts.statusCode)
			return
		}

		hijacker, ok := w.(http.Hijacker)
		if !ok {
			http.Error(w, "no hijack", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Connection", "Upgrade")
		w.Header().Set("Upgrade", "juju-ssh-relay")
		w.WriteHeader(http.StatusSwitchingProtocols)
		conn, _, err := hijacker.Hijack()
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		if rts.banner != "" {
			if _, err := conn.Write([]byte(rts.banner)); err != nil {
				return
			}
		}
		// Echo whatever the client sends so the returned connection can be
		// exercised end to end.
		_, _ = io.Copy(conn, conn)
	}))
	t.Cleanup(rts.server.Close)
	return rts
}

// tlsConfig returns a client TLS config that trusts the test server.
func (rts *relayTestServer) tlsConfig() *tls.Config {
	pool := x509.NewCertPool()
	pool.AddCert(rts.server.Certificate())
	return &tls.Config{RootCAs: pool}
}

// addr returns the host:port of the test server.
func (rts *relayTestServer) addr() string {
	return strings.TrimPrefix(rts.server.URL, "https://")
}

func TestBasicDialerDialRelaySuccess(t *testing.T) {
	c := qt.New(t)
	rts := newRelayTestServer(t)

	dialer := &ssh.BasicDialer{}
	conn, err := dialer.DialRelay(
		context.Background(),
		rts.addr(),
		rts.tlsConfig(),
		"1.postgresql.8419cd78-4993-4c3a-928e-c646226beeee.juju.local",
		"fake-bearer-token",
	)
	c.Assert(err, qt.IsNil)
	defer conn.Close()

	// The wire contract: path, upgrade token and bearer auth.
	c.Check(rts.gotPath, qt.Equals, "/ssh-relay/1.postgresql.8419cd78-4993-4c3a-928e-c646226beeee.juju.local")
	c.Check(rts.gotUpgrade, qt.Equals, "juju-ssh-relay")
	c.Check(rts.gotAuth, qt.Equals, "Bearer fake-bearer-token")

	// The connection is usable: what we write is echoed back.
	_, err = conn.Write([]byte("hello"))
	c.Assert(err, qt.IsNil)
	buf := make([]byte, 5)
	_, err = io.ReadFull(conn, buf)
	c.Assert(err, qt.IsNil)
	c.Check(string(buf), qt.Equals, "hello")
}

func TestBasicDialerDialRelayPreservesBufferedBytes(t *testing.T) {
	c := qt.New(t)
	rts := newRelayTestServer(t)
	// The server writes a banner immediately after the 101. The dialer's
	// bufio.Reader may consume these bytes past the response head, so they
	// must be preserved and served from the returned connection first.
	rts.banner = "SSH-2.0-Juju\r\n"

	dialer := &ssh.BasicDialer{}
	conn, err := dialer.DialRelay(
		context.Background(),
		rts.addr(),
		rts.tlsConfig(),
		"1.postgresql.8419cd78-4993-4c3a-928e-c646226beeee.juju.local",
		"fake-bearer-token",
	)
	c.Assert(err, qt.IsNil)
	defer conn.Close()

	// The banner must be the first thing read from the connection.
	buf := make([]byte, len(rts.banner))
	_, err = io.ReadFull(conn, buf)
	c.Assert(err, qt.IsNil)
	c.Check(string(buf), qt.Equals, rts.banner)
}

func TestBasicDialerDialRelayRejected(t *testing.T) {
	c := qt.New(t)
	rts := newRelayTestServer(t)
	rts.statusCode = http.StatusForbidden
	rts.rejectBody = "unauthorized"

	dialer := &ssh.BasicDialer{}
	conn, err := dialer.DialRelay(
		context.Background(),
		rts.addr(),
		rts.tlsConfig(),
		"1.postgresql.8419cd78-4993-4c3a-928e-c646226beeee.juju.local",
		"fake-bearer-token",
	)
	c.Assert(err, qt.IsNotNil)
	c.Check(conn, qt.IsNil)
	c.Check(err.Error(), qt.Contains, "relay upgrade rejected")
	c.Check(err.Error(), qt.Contains, "403")
	c.Check(err.Error(), qt.Contains, "unauthorized")
}

func TestBasicDialerDialRelayDialError(t *testing.T) {
	c := qt.New(t)

	dialer := &ssh.BasicDialer{}
	// Nothing is listening on this address, so the dial must fail.
	conn, err := dialer.DialRelay(
		context.Background(),
		"127.0.0.1:1",
		&tls.Config{InsecureSkipVerify: true}, //nolint:gosec // test only
		"1.postgresql.8419cd78-4993-4c3a-928e-c646226beeee.juju.local",
		"fake-bearer-token",
	)
	c.Assert(err, qt.IsNotNil)
	c.Check(conn, qt.IsNil)
	c.Check(err.Error(), qt.Contains, "dialing controller")
}

func TestSSHManager(t *testing.T) {
	qtsuite.Run(qt.New(t), &sshManagerSuite{})
}
