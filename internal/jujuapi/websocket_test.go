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

	"github.com/juju/juju/api"
	"github.com/juju/juju/rpc/jsoncodec"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v5"
	gc "gopkg.in/check.v1"

	"github.com/canonical/jimm/internal/dbmodel"
	"github.com/canonical/jimm/internal/jimmtest"
	"github.com/canonical/jimm/internal/jujuapi"
	"github.com/canonical/jimm/internal/openfga"
	ofganames "github.com/canonical/jimm/internal/openfga/names"
	"github.com/canonical/jimm/internal/wellknownapi"
)

type websocketSuite struct {
	jimmtest.BootstrapSuite

	Params     jujuapi.Params
	APIHandler http.Handler
	HTTP       *httptest.Server

	Credential2 *dbmodel.CloudCredential
	Model2      *dbmodel.Model
	Model3      *dbmodel.Model

	cancelFnc context.CancelFunc
}

func (s *websocketSuite) SetUpTest(c *gc.C) {
	ctx, cancelFnc := context.WithCancel(context.Background())
	s.cancelFnc = cancelFnc

	s.BootstrapSuite.SetUpTest(c)

	s.Params.ControllerUUID = "914487b5-60e7-42bb-bd63-1adc3fd3a388"

	mux := http.NewServeMux()
	mux.Handle("/api", jujuapi.APIHandler(ctx, s.JIMM, s.Params))
	mux.Handle("/model/", jujuapi.ModelHandler(ctx, s.JIMM, s.Params))
	jwks := wellknownapi.NewWellKnownHandler(s.JIMM.CredentialStore)
	mux.HandleFunc("/.well-known/jwks.json", jwks.JWKS)

	s.APIHandler = mux
	s.HTTP = httptest.NewTLSServer(s.APIHandler)

	s.AddAdminUser(c, "alice@canonical.com")

	cct := names.NewCloudCredentialTag(jimmtest.TestCloudName + "/charlie@canonical.com/cred")
	s.UpdateCloudCredential(c, cct, jujuparams.CloudCredential{AuthType: "empty"})
	s.Credential2 = new(dbmodel.CloudCredential)
	s.Credential2.SetTag(cct)
	err := s.JIMM.Database.GetCloudCredential(ctx, s.Credential2)
	c.Assert(err, gc.Equals, nil)

	mt := s.AddModel(c, names.NewUserTag("charlie@canonical.com"), "model-2", names.NewCloudTag(jimmtest.TestCloudName), jimmtest.TestCloudRegionName, cct)
	s.Model2 = new(dbmodel.Model)
	s.Model2.SetTag(mt)
	err = s.JIMM.Database.GetModel(ctx, s.Model2)
	c.Assert(err, gc.Equals, nil)

	mt = s.AddModel(c, names.NewUserTag("charlie@canonical.com"), "model-3", names.NewCloudTag(jimmtest.TestCloudName), jimmtest.TestCloudRegionName, cct)
	s.Model3 = new(dbmodel.Model)
	s.Model3.SetTag(mt)
	err = s.JIMM.Database.GetModel(ctx, s.Model3)
	c.Assert(err, gc.Equals, nil)

	bobIdentity, err := dbmodel.NewIdentity("bob@canonical.com")
	c.Assert(err, gc.IsNil)

	bob := openfga.NewUser(
		bobIdentity,
		s.OFGAClient,
	)
	err = bob.SetModelAccess(ctx, s.Model3.ResourceTag(), ofganames.ReaderRelation)
	c.Assert(err, gc.Equals, nil)
}

func (s *websocketSuite) TearDownTest(c *gc.C) {
	if s.cancelFnc != nil {
		s.cancelFnc()
	}
	if s.HTTP != nil {
		s.HTTP.Close()
	}
	s.BootstrapSuite.TearDownTest(c)
}

// openNoAssert creates a new websocket connection to the test server, using the
// connection info specified in info, authenticating as the given user.
// If info is nil then default values will be used.
func (s *websocketSuite) openNoAssert(
	c *gc.C,
	info *api.Info,
	username string,
	dialWebsocket func(ctx context.Context, urlStr string, tlsConfig *tls.Config, ipAddr string) (jsoncodec.JSONConn, error),
) (api.Connection, error) {
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

	lp := jimmtest.NewUserSessionLogin(c, username)

	dialOpts := api.DialOpts{
		InsecureSkipVerify: true,
		LoginProvider:      lp,
	}

	if dialWebsocket != nil {
		dialOpts.DialWebsocket = dialWebsocket
	}

	return api.Open(&inf, dialOpts)
}

func (s *websocketSuite) open(c *gc.C, info *api.Info, username string) api.Connection {
	conn, err := s.openNoAssert(c, info, username, nil)
	c.Assert(err, gc.Equals, nil)
	return conn
}

func (s *websocketSuite) openWithDialWebsocket(
	c *gc.C,
	info *api.Info,
	username string,
	dialWebsocket func(ctx context.Context, urlStr string, tlsConfig *tls.Config, ipAddr string) (jsoncodec.JSONConn, error),
) api.Connection {
	conn, err := s.openNoAssert(c, info, username, dialWebsocket)
	c.Assert(err, gc.Equals, nil)
	return conn
}

type proxySuite struct {
	websocketSuite
}

var _ = gc.Suite(&proxySuite{})

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

// TODO(CSS-7331) Refactor model proxy for new login methods
// func (s *proxySuite) TestConnectToModelAndLogin(c *gc.C) {
// 	ctx := context.Background()
// 	alice := names.NewUserTag("alice@canonical.com")
// 	aliceUser := openfga.NewUser(&dbmodel.Identity{Name: alice.Id()}, s.JIMM.OpenFGAClient)
// 	err := aliceUser.SetControllerAccess(ctx, s.Model.Controller.ResourceTag(), ofganames.AdministratorRelation)
// 	c.Assert(err, gc.IsNil)
// 	conn, err := s.openNoAssert(c, &api.Info{
// 		ModelTag:  s.Model.ResourceTag(),
// 		SkipLogin: false,
// 	}, "alice")
// 	if err == nil {
// 		defer conn.Close()
// 	}
// 	c.Assert(err, gc.Equals, nil)
// }

// TODO(CSS-7331) Add more tests for model proxy and new login methods.

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
