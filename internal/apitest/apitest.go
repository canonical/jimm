// Package apitest provides a test fixture for testing JEM APIs.
package apitest

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"time"

	jujufeature "github.com/juju/juju/feature"
	corejujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/testing/httptesting"
	"github.com/juju/utils/featureflag"
	gc "gopkg.in/check.v1"
	"gopkg.in/macaroon-bakery.v1/httpbakery"
	"gopkg.in/mgo.v2"

	external_jem "github.com/CanonicalLtd/jem"
	"github.com/CanonicalLtd/jem/internal/idmtest"
	"github.com/CanonicalLtd/jem/internal/jem"
	"github.com/CanonicalLtd/jem/jemclient"
	"github.com/CanonicalLtd/jem/params"
)

// Suite implements a test fixture that contains a JEM server
// and an identity discharging server.
type Suite struct {
	corejujutesting.JujuConnSuite

	// JEMSrv holds a running instance of JEM.
	JEMSrv *jem.Server

	// IDMSrv holds a running instance of the fake identity server.
	IDMSrv *idmtest.Server

	// httpSrv holds the running HTTP server that uses IDMSrv.
	httpSrv *httptest.Server
}

func (s *Suite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	s.PatchValue(&jem.APIOpenTimeout, time.Duration(0))
	s.IDMSrv = idmtest.NewServer()
	s.JEMSrv = s.NewServer(c, s.Session, s.IDMSrv)
	os.Setenv("JUJU_DEV_FEATURE_FLAGS", jujufeature.JES)
	featureflag.SetFlagsFromEnvironment("JUJU_DEV_FEATURE_FLAGS")
	s.httpSrv = httptest.NewServer(s.JEMSrv)
}

func (s *Suite) TearDownTest(c *gc.C) {
	s.httpSrv.Close()
	s.JEMSrv.Close()
	s.IDMSrv.Close()
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

// NewServer returns a new JEM server that uses the given mongo session and identity
// server.
func (s *Suite) NewServer(c *gc.C, session *mgo.Session, idmSrv *idmtest.Server) *jem.Server {
	db := session.DB("jem")
	s.IDMSrv.AddUser("agent")
	config := external_jem.ServerParams{
		DB:               db,
		StateServerAdmin: "admin",
		IdentityLocation: idmSrv.URL.String(),
		PublicKeyLocator: idmSrv,
		AgentUsername:    "agent",
		AgentKey:         s.IDMSrv.UserPublicKey("agent"),
	}
	srv, err := external_jem.NewServer(config)
	c.Assert(err, gc.IsNil)
	return srv.(*jem.Server)
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
