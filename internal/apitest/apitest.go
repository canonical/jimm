// Package apitest provides a test fixture for testing JEM APIs.
package apitest

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"time"

	"github.com/juju/idmclient"
	"github.com/juju/idmclient/idmtest"
	corejujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/testing/httptesting"
	gc "gopkg.in/check.v1"
	"gopkg.in/macaroon-bakery.v1/bakery"
	"gopkg.in/macaroon-bakery.v1/httpbakery"
	"gopkg.in/mgo.v2"

	external_jem "github.com/CanonicalLtd/jem"
	"github.com/CanonicalLtd/jem/internal/jem"
	"github.com/CanonicalLtd/jem/internal/jemserver"
	"github.com/CanonicalLtd/jem/jemclient"
	"github.com/CanonicalLtd/jem/params"
)

// Suite implements a test fixture that contains a JEM server
// and an identity discharging server.
type Suite struct {
	corejujutesting.JujuConnSuite

	// JEMSrv holds a running instance of JEM.
	JEMSrv *jemserver.Server

	// IDMSrv holds a running instance of the fake identity server.
	IDMSrv *idmtest.Server

	// httpSrv holds the running HTTP server that uses IDMSrv.
	httpSrv *httptest.Server

	// JEM holds an instance of the JEM store, suitable
	// for invasive testing purposes.
	JEM *jem.JEM

	// Pool holds the pool from which the above JEM was taken.
	Pool *jem.Pool
}

func (s *Suite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	s.PatchValue(&jem.APIOpenTimeout, time.Duration(0))
	s.IDMSrv = idmtest.NewServer()
	s.JEMSrv = s.NewServer(c, s.Session, s.IDMSrv)
	s.httpSrv = httptest.NewServer(s.JEMSrv)

	s.Pool = s.newPool(c, s.Session)
	s.JEM = s.Pool.JEM()
}

func (s *Suite) newPool(c *gc.C, session *mgo.Session) *jem.Pool {
	pool, err := jem.NewPool(jem.Params{
		DB: session.DB("jem"),
		BakeryParams: bakery.NewServiceParams{
			Location: "here",
		},
		IDMClient: idmclient.New(idmclient.NewParams{
			BaseURL: s.IDMSrv.URL.String(),
			Client:  s.IDMSrv.Client("agent"),
		}),
		ControllerAdmin: "controller-admin",
	})
	c.Assert(err, gc.IsNil)
	return pool
}

func (s *Suite) TearDownTest(c *gc.C) {
	s.httpSrv.Close()
	s.JEMSrv.Close()
	s.IDMSrv.Close()
	s.JEM.Close()
	s.Pool.Close()
	s.JujuConnSuite.TearDownTest(c)
}

// NewClient returns a new JEM client that is configured to talk to
// s.JEMSrv.
func (s *Suite) NewClient(username params.User) *jemclient.Client {
	return jemclient.New(jemclient.NewParams{
		BaseURL: s.httpSrv.URL,
		Client:  s.IDMSrv.Client(string(username)),
	})
}

// ProxiedPool returns a JEM pool that uses a proxied TCP connection
// to MongoDB and the proxy that the connections go through.
// This makes it possible to test what happens when a connection
// to the database is broken.
//
// Both the returned pool and the returned proxy should
// be closed after use.
func (s *Suite) ProxiedPool(c *gc.C) (*jem.Pool, *testing.TCPProxy) {
	mgoInfo := testing.MgoServer.DialInfo()
	c.Assert(mgoInfo.Addrs, gc.HasLen, 1)
	proxy := testing.NewTCPProxy(c, mgoInfo.Addrs[0])
	mgoInfo.Addrs = []string{proxy.Addr()}
	session, err := mgo.DialWithInfo(mgoInfo)
	c.Assert(err, gc.IsNil)
	return s.newPool(c, session), proxy
}

// NewServer returns a new JEM server that uses the given mongo session and identity
// server.
func (s *Suite) NewServer(c *gc.C, session *mgo.Session, idmSrv *idmtest.Server) *jemserver.Server {
	db := session.DB("jem")
	s.IDMSrv.AddUser("agent")
	config := external_jem.ServerParams{
		DB:               db,
		ControllerAdmin:  "controller-admin",
		IdentityLocation: idmSrv.URL.String(),
		PublicKeyLocator: idmSrv,
		AgentUsername:    "agent",
		AgentKey:         s.IDMSrv.UserPublicKey("agent"),
	}
	srv, err := external_jem.NewServer(config)
	c.Assert(err, gc.IsNil)
	return srv.(*jemserver.Server)
}

// AssertAddController adds the specified controller using AddController
// and checks that id succeeds. It returns the controller id.
func (s *Suite) AssertAddController(c *gc.C, path params.EntityPath, loc map[string]string) params.EntityPath {
	err := s.AddController(c, path, loc)
	c.Assert(err, jc.ErrorIsNil)
	return path
}

// AddController adds a new controller with the provided path and any
// specified location parameters.
func (s *Suite) AddController(c *gc.C, path params.EntityPath, loc map[string]string) error {
	// Note that because the cookies acquired in this request don't
	// persist, the discharge macaroon we get won't affect subsequent
	// requests in the caller.
	info := s.APIInfo(c)
	p := &params.AddController{
		EntityPath: path,
		Info: params.ControllerInfo{
			HostPorts:      info.Addrs,
			CACert:         info.CACert,
			User:           info.Tag.Id(),
			Password:       info.Password,
			ControllerUUID: info.ModelTag.Id(),
			Location:       loc,
		},
	}
	// If locations are specified then make the controller public so
	// that it can be found by seraching on those locations.
	if len(loc) > 0 {
		p.Info.Public = true
		s.IDMSrv.AddUser(string(path.User), "controller-admin")
	}
	return s.NewClient(path.User).AddController(p)
}

const dummySSHKey = "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQDOjaOjVRHchF2RFCKQdgBqrIA5nOoqSprLK47l2th5I675jw+QYMIihXQaITss3hjrh3+5ITyBO41PS5rHLNGtlYUHX78p9CHNZsJqHl/z1Ub1tuMe+/5SY2MkDYzgfPtQtVsLasAIiht/5g78AMMXH3HeCKb9V9cP6/lPPq6mCMvg8TDLrPp/P2vlyukAsJYUvVgoaPDUBpedHbkMj07pDJqe4D7c0yEJ8hQo/6nS+3bh9Q1NvmVNsB1pbtk3RKONIiTAXYcjclmOljxxJnl1O50F5sOIi38vyl7Q63f6a3bXMvJEf1lnPNJKAxspIfEu8gRasny3FEsbHfrxEwVj rog@rog-x220"

var dummyModelConfig = map[string]interface{}{
	"authorized-keys": dummySSHKey,
	"controller":      true,
}

// CreateModel creates a new model with the specified path on the
// specified controller, using the specified templates. It returns the
// new model's path, user and uuid.
func (s *Suite) CreateModel(c *gc.C, path, ctlPath params.EntityPath, templates ...params.EntityPath) (modelPath params.EntityPath, user, uuid string) {
	// Note that because the cookies acquired in this request don't
	// persist, the discharge macaroon we get won't affect subsequent
	// requests in the caller.
	resp, err := s.NewClient(path.User).NewModel(&params.NewModel{
		User: path.User,
		Info: params.NewModelInfo{
			Name:          path.Name,
			Controller:    &ctlPath,
			Config:        dummyModelConfig,
			TemplatePaths: templates,
		},
	})
	c.Assert(err, gc.IsNil)
	return resp.Path, resp.User, resp.UUID
}

// Do returns a Do function appropriate for using in httptesting.AssertJSONCall.Do
// that makes its HTTP request acting as the given client.
// If client is nil, it uses httpbakery.NewClient instead.
//
// This can be used to cause the HTTP request to act as an
// arbitrary user.
func Do(client *httpbakery.Client) func(*http.Request) (*http.Response, error) {
	if client == nil {
		client = httpbakery.NewClient()
	}
	return func(req *http.Request) (*http.Response, error) {
		if req.Body != nil {
			body := req.Body.(io.ReadSeeker)
			req.Body = nil
			return client.DoWithBody(req, body)
		}
		return client.Do(req)
	}
}

// AnyBody is a convenience value that can be used in
// httptesting.AssertJSONCall.ExpectBody to cause
// AssertJSONCall to ignore the contents of the response body.
var AnyBody = httptesting.BodyAsserter(func(*gc.C, json.RawMessage) {})
