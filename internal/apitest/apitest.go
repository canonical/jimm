// Package apitest provides a test fixture for testing JEM APIs.
package apitest

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"time"

	"github.com/juju/idmclient"
	corejujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/testing"
	"github.com/juju/testing/httptesting"
	gc "gopkg.in/check.v1"
	"gopkg.in/macaroon-bakery.v1/bakery"
	"gopkg.in/macaroon-bakery.v1/httpbakery"
	"gopkg.in/mgo.v2"

	external_jem "github.com/CanonicalLtd/jem"
	"github.com/CanonicalLtd/jem/internal/idmtest"
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
		ControllerAdmin:  "admin",
		IdentityLocation: idmSrv.URL.String(),
		PublicKeyLocator: idmSrv,
		AgentUsername:    "agent",
		AgentKey:         s.IDMSrv.UserPublicKey("agent"),
	}
	srv, err := external_jem.NewServer(config)
	c.Assert(err, gc.IsNil)
	return srv.(*jemserver.Server)
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
