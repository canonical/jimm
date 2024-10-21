// Copyright 2024 Canonical.

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
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/juju/juju/api"
	"github.com/juju/juju/api/client/client"
	"github.com/juju/juju/rpc/jsoncodec"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v5"
	gc "gopkg.in/check.v1"

	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/jimmhttp"
	"github.com/canonical/jimm/v3/internal/jujuapi"
	"github.com/canonical/jimm/v3/internal/openfga"
	ofganames "github.com/canonical/jimm/v3/internal/openfga/names"
	"github.com/canonical/jimm/v3/internal/testutils/jimmtest"
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

	s.Params.ControllerUUID = jimmtest.ControllerUUID

	mux := chi.NewRouter()
	mountHandler := func(path string, h jimmhttp.JIMMHttpHandler) {
		mux.Mount(path, h.Routes())
	}
	mux.Handle("/api", jujuapi.APIHandler(ctx, s.JIMM, s.Params))
	mountHandler(
		"/model/{uuid}/{type:charms|applications}",
		jimmhttp.NewHTTPProxyHandler(s.JIMM),
	)
	mux.Handle("/model/*", http.StripPrefix("/model", jujuapi.ModelHandler(ctx, s.JIMM, s.Params)))
	jwks := jimmhttp.NewWellKnownHandler(s.JIMM.CredentialStore)
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

type loginDetails struct {
	info          *api.Info
	username      string
	lp            api.LoginProvider
	dialWebsocket func(ctx context.Context, urlStr string, tlsConfig *tls.Config, ipAddr string) (jsoncodec.JSONConn, error)
}

// openNoAssert creates a new websocket connection to the test server, using the
// connection info specified in info, authenticating as the given user.
// If info is nil then default values will be used.
func (s *websocketSuite) openNoAssert(c *gc.C, d loginDetails) (api.Connection, error) {
	var inf api.Info
	if d.info != nil {
		inf = *d.info
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

	if d.lp == nil {
		d.lp = jimmtest.NewUserSessionLogin(c, d.username)
	}

	dialOpts := api.DialOpts{
		InsecureSkipVerify: true,
		LoginProvider:      d.lp,
	}

	if d.dialWebsocket != nil {
		dialOpts.DialWebsocket = d.dialWebsocket
	}

	return api.Open(&inf, dialOpts)
}

func (s *websocketSuite) open(c *gc.C, info *api.Info, username string) api.Connection {
	ld := loginDetails{info: info, username: username}
	conn, err := s.openNoAssert(c, ld)
	c.Assert(err, gc.Equals, nil)
	return conn
}

func (s *websocketSuite) openCustomLoginProvider(c *gc.C, info *api.Info, username string, lp api.LoginProvider) (api.Connection, error) {
	ld := loginDetails{info: info, username: username, lp: lp}
	return s.openNoAssert(c, ld)
}

func (s *websocketSuite) openWithDialWebsocket(
	c *gc.C,
	info *api.Info,
	username string,
	dialWebsocket func(ctx context.Context, urlStr string, tlsConfig *tls.Config, ipAddr string) (jsoncodec.JSONConn, error),
) api.Connection {
	ld := loginDetails{info: info, username: username, dialWebsocket: dialWebsocket}
	conn, err := s.openNoAssert(c, ld)
	c.Assert(err, gc.Equals, nil)
	return conn
}

type apiProxySuite struct {
	websocketSuite
}

var _ = gc.Suite(&apiProxySuite{})

func (s *apiProxySuite) TestConnectToModel(c *gc.C) {
	conn := s.open(c, &api.Info{
		ModelTag:  s.Model.ResourceTag(),
		SkipLogin: true,
	}, "test")
	defer conn.Close()
	var resp map[string]interface{}
	err := conn.APICall("Admin", 3, "", "TestMethod", nil, &resp)
	c.Assert(err, gc.ErrorMatches, `no such request - method Admin.TestMethod is not implemented \(not implemented\)`)
}

// TestSessionTokenLoginProvider verifies that the session token login provider works as expected.
// We do this by using a mock authenticator that simulates polling an OIDC server and verifying that
// the user would be prompted with a login URL and fake the user login via the `EnableDeviceFlow` method.
func (s *apiProxySuite) TestSessionTokenLoginProvider(c *gc.C) {
	ctx := context.Background()
	alice := names.NewUserTag("alice@canonical.com")
	aliceUser := openfga.NewUser(&dbmodel.Identity{Name: alice.Id()}, s.JIMM.OpenFGAClient)
	err := aliceUser.SetControllerAccess(ctx, s.Model.Controller.ResourceTag(), ofganames.AdministratorRelation)
	c.Assert(err, gc.IsNil)
	var output bytes.Buffer
	s.JIMMSuite.EnableDeviceFlow(aliceUser.Name)
	conn, err := s.openCustomLoginProvider(c, &api.Info{
		ModelTag:  s.Model.ResourceTag(),
		SkipLogin: false,
	}, "alice", api.NewSessionTokenLoginProvider("", &output, func(s string) error { return nil }))
	c.Assert(err, gc.IsNil)
	defer conn.Close()
	c.Check(err, gc.Equals, nil)
	outputNoNewLine := strings.ReplaceAll(output.String(), "\n", "")
	c.Check(outputNoNewLine, gc.Matches, `Please visit .* and enter code.*`)
}

type logger struct{}

func (l logger) Errorf(string, ...interface{}) {}

func (s *apiProxySuite) TestModelStatus(c *gc.C) {
	conn := s.open(c, &api.Info{
		ModelTag:  s.Model.ResourceTag(),
		SkipLogin: false,
	}, "alice@canonical.com")
	defer conn.Close()
	jujuClient := client.NewClient(conn, logger{})
	status, err := jujuClient.Status(nil)
	c.Check(err, gc.IsNil)
	c.Check(status, gc.Not(gc.IsNil))
	c.Check(status.Model.Name, gc.Equals, s.Model.Name)
}

func (s *apiProxySuite) TestModelStatusWithoutPermission(c *gc.C) {
	fooUser := openfga.NewUser(&dbmodel.Identity{Name: "foo@canonical.com"}, s.JIMM.OpenFGAClient)
	var output bytes.Buffer
	s.JIMMSuite.EnableDeviceFlow(fooUser.Name)
	conn, err := s.openCustomLoginProvider(c, &api.Info{
		ModelTag:  s.Model.ResourceTag(),
		SkipLogin: false,
	}, "foo", api.NewSessionTokenLoginProvider("", &output, func(s string) error { return nil }))
	c.Check(err, gc.ErrorMatches, "permission denied .*")
	if conn != nil {
		defer conn.Close()
	}
	outputNoNewLine := strings.ReplaceAll(output.String(), "\n", "")
	c.Check(outputNoNewLine, gc.Matches, `Please visit .* and enter code.*`)
}

// TODO(Kian): This test aims to verify that JIMM gracefully handles clients that end their connection
// during the login flow after JIMM starts polling the OIDC server.
// After https://github.com/juju/juju/pull/17606 lands we can begin work on this.
// The API connection's login method should be refactored use the login provider stored on the state struct.
// func (s *apiProxySuite) TestDeviceFlowCancelDuringPolling(c *gc.C) {
// 	ctx := context.Background()
// 	alice := names.NewUserTag("alice@canonical.com")
// 	aliceUser := openfga.NewUser(&dbmodel.Identity{Name: alice.Id()}, s.JIMM.OpenFGAClient)
// 	err := aliceUser.SetControllerAccess(ctx, s.Model.Controller.ResourceTag(), ofganames.AdministratorRelation)
// 	c.Assert(err, gc.IsNil)
// 	var cliOutput string
// 	_ = cliOutput
// 	outputFunc := func(format string, a ...any) error {
// 		cliOutput = fmt.Sprintf(format, a)
// 		return nil
// 	}
// 	var wg sync.WaitGroup
// 	var conn api.Connection
// 	wg.Add(1)
// 	go func() {
// 		defer wg.Done()
// 		conn, err = s.openCustomLP(c, &api.Info{
// 			ModelTag:  s.Model.ResourceTag(),
// 			SkipLogin: true,
// 		}, "alice", api.NewSessionTokenLoginProvider("", outputFunc, func(s string) error { return nil }))
// 		c.Assert(err, gc.IsNil)
// 	}()
// 	conn.Login()
//  // Close the connection after the cliOutput is filled.
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
		{path: fmt.Sprintf("/%s/api", testUUID), uuid: testUUID, finalPath: "api", fail: false},
		{path: fmt.Sprintf("/%s/api/", testUUID), uuid: testUUID, finalPath: "api/", fail: false},
		{path: fmt.Sprintf("/%s/api/foo", testUUID), uuid: testUUID, finalPath: "api/foo", fail: false},
		{path: fmt.Sprintf("/%s/commands", testUUID), uuid: testUUID, finalPath: "commands", fail: false},
		{path: fmt.Sprintf("%s/commands", testUUID), fail: true},
		{path: fmt.Sprintf("/model/%s/commands", testUUID), fail: true},
		{path: "/model/123/commands", fail: true},
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
