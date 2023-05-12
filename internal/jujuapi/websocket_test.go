// Copyright 2016 Canonical Ltd.

package jujuapi_test

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"time"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/httpbakery"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/httpbakery/agent"
	"github.com/juju/juju/api"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v4"
	"github.com/juju/zaputil/zapctx"
	"go.uber.org/zap"
	gc "gopkg.in/check.v1"

	"github.com/CanonicalLtd/jimm/internal/dbmodel"
	"github.com/CanonicalLtd/jimm/internal/jimmjwx"
	"github.com/CanonicalLtd/jimm/internal/jimmtest"
	"github.com/CanonicalLtd/jimm/internal/jujuapi"
	"github.com/CanonicalLtd/jimm/internal/openfga"
	ofganames "github.com/CanonicalLtd/jimm/internal/openfga/names"
	"github.com/CanonicalLtd/jimm/internal/wellknownapi"
)

type websocketSuite struct {
	jimmtest.CandidSuite
	jimmtest.BootstrapSuite

	InMemoryStore jimmtest.InMemoryCredentialStore
	Params        jujuapi.Params
	APIHandler    http.Handler
	HTTP          *httptest.Server

	Credential2 *dbmodel.CloudCredential
	Model2      *dbmodel.Model
	Model3      *dbmodel.Model
}

func (s *websocketSuite) SetUpTest(c *gc.C) {
	ctx := context.Background()

	s.ControllerAdmins = []string{"controller-admin"}

	s.CandidSuite.SetUpTest(c)
	s.BootstrapSuite.SetUpTest(c)

	s.JIMM.Authenticator = s.Authenticator
	s.JIMM.JWKService = jimmjwx.NewJWKSService(&s.InMemoryStore)

	s.Params.ControllerUUID = "914487b5-60e7-42bb-bd63-1adc3fd3a388"
	s.Params.IdentityLocation = s.Candid.URL.String()

	mux := http.NewServeMux()
	mux.Handle("/api", jujuapi.APIHandler(ctx, s.JIMM, s.Params))
	mux.Handle("/model/", jujuapi.ModelHandler(ctx, s.JIMM, s.Params))
	jwks := wellknownapi.NewWellKnownHandler(&s.InMemoryStore)
	mux.HandleFunc("/.well-known/jwks.json", jwks.JWKS)

	s.APIHandler = mux
	s.HTTP = httptest.NewTLSServer(s.APIHandler)

	s.Candid.AddUser("alice")

	cct := names.NewCloudCredentialTag(jimmtest.TestCloudName + "/charlie@external/cred")
	s.UpdateCloudCredential(c, cct, jujuparams.CloudCredential{AuthType: "empty"})
	s.Credential2 = new(dbmodel.CloudCredential)
	s.Credential2.SetTag(cct)
	err := s.JIMM.Database.GetCloudCredential(ctx, s.Credential2)
	c.Assert(err, gc.Equals, nil)

	mt := s.AddModel(c, names.NewUserTag("charlie@external"), "model-2", names.NewCloudTag(jimmtest.TestCloudName), jimmtest.TestCloudRegionName, cct)
	s.Model2 = new(dbmodel.Model)
	s.Model2.SetTag(mt)
	err = s.JIMM.Database.GetModel(ctx, s.Model2)
	c.Assert(err, gc.Equals, nil)

	mt = s.AddModel(c, names.NewUserTag("charlie@external"), "model-3", names.NewCloudTag(jimmtest.TestCloudName), jimmtest.TestCloudRegionName, cct)
	s.Model3 = new(dbmodel.Model)
	s.Model3.SetTag(mt)
	err = s.JIMM.Database.GetModel(ctx, s.Model3)
	c.Assert(err, gc.Equals, nil)

	// TODO (alesstimec) granting model access will be implemented in a followup
	//conn := s.open(c, nil, "charlie")
	//defer conn.Close()
	//client := modelmanager.NewClient(conn)
	//
	//err = client.GrantModel("bob@external", "read", mt.Id())
	//c.Assert(err, gc.Equals, nil)

	bob := openfga.NewUser(
		&dbmodel.User{
			Username: "bob@external",
		},
		s.OFGAClient,
	)
	err = bob.SetModelAccess(context.Background(), s.Model3.ResourceTag(), ofganames.ReaderRelation)
	c.Assert(err, gc.Equals, nil)
}

func (s *websocketSuite) TearDownTest(c *gc.C) {
	s.BootstrapSuite.TearDownTest(c)
	s.CandidSuite.TearDownTest(c)
}

// openNoAssert creates a new websocket connection to the test server, using the
// connection info specified in info, authenticating as the given user.
// If info is nil then default values will be used.
func (s *websocketSuite) openNoAssert(c *gc.C, info *api.Info, username string) (api.Connection, error) {
	var inf api.Info
	if info != nil {
		inf = *info
	}
	u, err := url.Parse(s.HTTP.URL)
	c.Assert(err, gc.Equals, nil)
	inf.Addrs = []string{
		u.Host,
	}
	w := new(bytes.Buffer)
	err = pem.Encode(w, &pem.Block{
		Type:  "CERTIFICATE",
		Bytes: s.HTTP.TLS.Certificates[0].Certificate[0],
	})
	c.Assert(err, gc.Equals, nil)
	inf.CACert = w.String()

	s.Candid.AddUser(username)
	key := s.Candid.UserPublicKey(username)
	bClient := httpbakery.NewClient()
	bClient.Key = &bakery.KeyPair{
		Public:  bakery.PublicKey{Key: bakery.Key(key.Public.Key)},
		Private: bakery.PrivateKey{Key: bakery.Key(key.Private.Key)},
	}
	agent.SetUpAuth(bClient, &agent.AuthInfo{
		Key: bClient.Key,
		Agents: []agent.Agent{{
			URL:      s.Candid.URL.String(),
			Username: username,
		}},
	})

	return api.Open(&inf, api.DialOpts{
		InsecureSkipVerify: true,
		BakeryClient:       bClient,
	})
}

func (s *websocketSuite) open(c *gc.C, info *api.Info, username string) api.Connection {
	conn, err := s.openNoAssert(c, info, username)
	c.Assert(err, gc.Equals, nil)
	return conn
}

type proxySuite struct {
	websocketSuite
	cancelJwkRotator context.CancelFunc
}

var _ = gc.Suite(&proxySuite{})

func (s *proxySuite) SetUpTest(c *gc.C) {
	s.websocketSuite.SetUpTest(c)
	ctx, cancel := context.WithCancel(context.Background())
	s.cancelJwkRotator = cancel
	// This suite sets up the JWT service which allows JIMM to mint JWTs
	// We don't set it up in the websocket suite to speed up tests that don't need it.
	go func() error {
		return s.JIMM.JWKService.StartJWKSRotator(ctx, time.NewTicker(time.Hour).C, time.Now().UTC().AddDate(0, 3, 0))
	}()
	zapctx.Debug(ctx, "URL", zap.String("URL", s.HTTP.URL))
	url, err := url.Parse(s.HTTP.URL)
	c.Assert(err, gc.IsNil)
	c.Assert(os.Setenv("JIMM_JWT_EXPIRY", "30s"), gc.IsNil)
	c.Assert(os.Setenv("JIMM_DNS_NAME", url.Host), gc.IsNil)
	s.JIMM.JWTService = jimmjwx.NewJWTService(url.Host, &s.InMemoryStore, true)
	tlsConfig := &tls.Config{
		InsecureSkipVerify: true,
	}
	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: tlsConfig,
		},
		Timeout: 15 * time.Second,
	}
	s.JIMM.JWTService.RegisterJWKSCache(ctx, client)
}

func (s *proxySuite) TearDownTest(c *gc.C) {
	os.Clearenv()
	if s.cancelJwkRotator != nil {
		s.cancelJwkRotator()
	}
	s.websocketSuite.TearDownTest(c)
}

func (s *proxySuite) TestConnectToModel(c *gc.C) {
	conn := s.open(c, &api.Info{
		ModelTag:  s.Model.ResourceTag(),
		SkipLogin: true,
	}, "test")
	defer conn.Close()
	var resp map[string]interface{}
	err := conn.APICall("Admin", 3, "", "TestMethod", nil, &resp)
	c.Assert(err, gc.ErrorMatches, `no such request - method Admin.TestMethod is not implemented \(not implemented\)`)
}

// TODO(Kian): Once JIMM Tests run against Juju 3.2 this test should no longer return an error.
// This tests makes a connection to the proxy service, mints a JWT and passes the modified login
// request to the controller.
func (s *proxySuite) TestConnectToModelAndLogin(c *gc.C) {
	ctx := context.Background()
	alice := names.NewUserTag("alice")
	aliceUser := openfga.NewUser(&dbmodel.User{Username: alice.Id()}, s.JIMM.OpenFGAClient)
	err := aliceUser.SetControllerAccess(ctx, s.Model.Controller.ResourceTag(), ofganames.AdministratorRelation)
	c.Assert(err, gc.IsNil)
	conn, err := s.openNoAssert(c, &api.Info{
		ModelTag:  s.Model.ResourceTag(),
		SkipLogin: false,
	}, "alice")
	if err == nil {
		defer conn.Close()
	}
	c.Assert(err, gc.ErrorMatches, `parsing request authToken: no jwt authToken parser configured`)
}

// TestConnectToModelNoBakeryClient ensures that authentication is in fact
// happening, without a bakery client the test should see an error from Candid.
func (s *proxySuite) TestConnectToModelNoBakeryClient(c *gc.C) {
	inf := api.Info{
		ModelTag:  s.Model.ResourceTag(),
		SkipLogin: false,
	}
	u, err := url.Parse(s.HTTP.URL)
	c.Assert(err, gc.Equals, nil)
	inf.Addrs = []string{
		u.Host,
	}
	c.Assert(err, gc.Equals, nil)
	_, err = api.Open(&inf, api.DialOpts{
		InsecureSkipVerify: true,
		BakeryClient:       nil,
	})
	c.Assert(err, gc.ErrorMatches, "interaction required but not possible")
}

type pathTestSuite struct{}

var _ = gc.Suite(&pathTestSuite{})

func (s *pathTestSuite) Test(c *gc.C) {

	testUUID := "059744f6-26d2-4f00-92be-5df97fccbb97"
	tests := []struct {
		path      string
		uuid      string
		finalPath string
		fail      bool
	}{
		{path: fmt.Sprintf("/model/%s/api", testUUID), uuid: testUUID, finalPath: "api", fail: false},
		{path: fmt.Sprintf("model/%s/api", testUUID), uuid: testUUID, finalPath: "api", fail: false},
		{path: fmt.Sprintf("/model/%s/api/", testUUID), uuid: testUUID, finalPath: "api/", fail: false},
		{path: fmt.Sprintf("/model/%s/api/foo", testUUID), uuid: testUUID, finalPath: "api/foo", fail: false},
		{path: fmt.Sprintf("/model/%s/commands", testUUID), uuid: testUUID, finalPath: "commands", fail: false},
		{path: "/model/123/commands", uuid: "123", finalPath: "commands", fail: true},
		{path: fmt.Sprintf("/controller/%s/commands", testUUID), fail: true},
		{path: fmt.Sprintf("/controller/%s/", testUUID), fail: true},
		{path: "/controller", fail: true},
	}
	for i, test := range tests {
		c.Logf("Running test %d for path %s", i, test.path)
		uuid, finalPath, err := jujuapi.ModelInfoFromPath(test.path)
		if !test.fail {
			c.Assert(err, gc.IsNil)
			c.Assert(uuid, gc.Equals, test.uuid)
			c.Assert(finalPath, gc.Equals, test.finalPath)
		} else {
			c.Assert(err, gc.NotNil)
		}
	}
}
