// Copyright 2025 Canonical.

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

	"github.com/go-chi/chi/v5"
	"github.com/juju/juju/api"
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
	mux.Handle("/migrate/logtransfer", jujuapi.LogTransferHandler(ctx, s.JIMM, s.Params))

	s.APIHandler = mux
	s.HTTP = httptest.NewTLSServer(s.APIHandler)

	s.AddAdminUser(c, "alice@canonical.com")

	cct := names.NewCloudCredentialTag(jimmtest.TestCloudName + "/charlie@canonical.com/cred")
	s.UpdateCloudCredential(c, cct, jujuparams.CloudCredential{AuthType: "empty"})
	s.Credential2 = new(dbmodel.CloudCredential)
	s.Credential2.SetTag(cct)
	err := s.JIMM.Database.GetCloudCredential(ctx, s.Credential2)
	c.Assert(err, gc.Equals, nil)

	// Grant Charlie add-model access to controller-1
	controller := dbmodel.Controller{Name: "controller-1"}
	err = s.JIMM.Database.GetController(ctx, &controller)
	c.Assert(err, gc.Equals, nil)
	err = s.OFGAClient.AddRelation(ctx, openfga.Tuple{
		Object:   ofganames.ConvertTag(names.NewUserTag("charlie@canonical.com")),
		Relation: ofganames.CanAddModelRelation,
		Target:   ofganames.ConvertTag(controller.ResourceTag()),
	})
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
