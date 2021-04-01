package apitest

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"

	"github.com/canonical/candid/candidtest"
	bakeryv3 "github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	httpbakeryv3 "github.com/go-macaroon-bakery/macaroon-bakery/v3/httpbakery"
	agentv3 "github.com/go-macaroon-bakery/macaroon-bakery/v3/httpbakery/agent"
	"github.com/juju/aclstore"
	controllerapi "github.com/juju/juju/api/controller"
	"github.com/juju/juju/controller"
	"github.com/juju/simplekv/mgosimplekv"
	"github.com/juju/testing"
	"github.com/juju/testing/httptesting"
	"github.com/julienschmidt/httprouter"
	gc "gopkg.in/check.v1"
	"gopkg.in/errgo.v1"
	"gopkg.in/httprequest.v1"
	"gopkg.in/macaroon-bakery.v2/bakery"
	"gopkg.in/macaroon-bakery.v2/bakery/identchecker"
	"gopkg.in/macaroon-bakery.v2/httpbakery"

	"github.com/CanonicalLtd/jimm/internal/auth"
	"github.com/CanonicalLtd/jimm/internal/jemerror"
	"github.com/CanonicalLtd/jimm/internal/jemserver"
	"github.com/CanonicalLtd/jimm/internal/jemtest"
)

const ControllerUUID = "8e8e17d2-9267-489e-9850-767e317fe482"

// APISuite is a mixin to help testing API handlers.
type APISuite struct {
	testing.CleanupSuite
	// Params used to initialise the API handler. The following fields must
	// be specified before calling SetUpTest:
	//
	//     JEMPool
	//     SessionPool
	//
	// Additionally the following fields will be initialized in SetUpTest
	// if they contain zero values:
	//
	//    ACLManager
	//    AgentKey
	//    AgentUsername
	//    Authenticator
	//    ControllerAdmin
	//    ControllerUUID
	//    IdentityLocation
	//    ThirdPartyLocator
	Params jemserver.HandlerParams

	// NewAPIHandlerFunc is used to initialise the API handler for the
	// tests.
	NewAPIHandler jemserver.NewAPIHandlerFunc

	// UseTLS configures the HTTP server to serve using TLS.
	UseTLS bool

	// The following fields will be populated following SetUpSuite.

	// Candid holds a running instance of the fake identity server.
	Candid *candidtest.Server

	// The following fields will be populated following SetUpTest.

	// ACLStore holds the store for ACLs
	ACLStore aclstore.ACLStore

	// APIHandler holds a http.Handler created from calling NewAPIHandler.
	APIHandler http.Handler

	// HTTP holds a HTTP server serving APIHandler.
	HTTP *httptest.Server
}

func (s *APISuite) SetUpSuite(c *gc.C) {
	s.CleanupSuite.SetUpSuite(c)
	s.Candid = candidtest.NewServer()
}

func (s *APISuite) TearDownSuite(c *gc.C) {
	if s.Candid != nil {
		s.Candid.Close()
		s.Candid = nil
	}
	s.CleanupSuite.TearDownSuite(c)
}

func (s *APISuite) ConfigureController(jcs *jemtest.JujuConnSuite) {
	jcs.ControllerConfigAttrs = map[string]interface{}{
		controller.IdentityURL:         s.Candid.URL,
		controller.IdentityPublicKey:   s.Candid.PublicKey,
		controller.AllowModelAccessKey: true,
	}
}

func (s *APISuite) SetUpTest(c *gc.C) {
	s.CleanupSuite.SetUpTest(c)
	ctx := context.Background()

	jem := s.Params.JEMPool.JEM(ctx)
	defer jem.Close()

	if s.Params.IdentityLocation == "" {
		s.Params.IdentityLocation = s.Candid.URL.String()
		s.AddCleanup(func(c *gc.C) {
			s.Params.IdentityLocation = ""
		})
	}

	if s.Params.ThirdPartyLocator == nil {
		s.Params.ThirdPartyLocator = s.Candid
		s.AddCleanup(func(c *gc.C) {
			s.Params.ThirdPartyLocator = nil
		})
	}

	if s.Params.AgentUsername == "" {
		s.Params.AgentUsername = "jem-test-agent"
		s.AddCleanup(func(c *gc.C) {
			s.Params.AgentUsername = ""
		})
	}
	s.Candid.AddUser(s.Params.AgentUsername, candidtest.GroupListGroup)
	if s.Params.AgentKey == nil {
		s.Params.AgentKey = s.Candid.UserPublicKey(s.Params.AgentUsername)
		s.AddCleanup(func(c *gc.C) {
			s.Params.AgentKey = nil
		})
	}

	if s.Params.Authenticator == nil {
		key, err := bakery.GenerateKey()
		c.Assert(err, gc.Equals, nil)
		bakery := identchecker.NewBakery(identchecker.BakeryParams{
			Locator:        s.Params.ThirdPartyLocator,
			Key:            key,
			IdentityClient: s.Candid.CandidClient(s.Params.AgentUsername),
			Authorizer: identchecker.ACLAuthorizer{
				GetACL: func(ctx context.Context, op bakery.Op) (acl []string, allowPublic bool, err error) {
					if op == identchecker.LoginOp {
						return []string{identchecker.Everyone}, false, nil
					}
					return nil, false, nil
				},
			},
			Location: "jimmtest",
			Logger:   bakeryLogger{c: c},
		})
		s.Params.Authenticator = auth.NewAuthenticator(bakery)
		s.AddCleanup(func(c *gc.C) {
			s.Params.Authenticator = nil
		})
	}

	if s.Params.ACLManager == nil {
		session := jem.DB.Session.Copy()
		s.AddCleanup(func(c *gc.C) { session.Close() })
		kvstore, err := mgosimplekv.NewStore(jem.DB.C("acls").With(session))
		c.Assert(err, gc.Equals, nil)
		s.ACLStore = aclstore.NewACLStore(kvstore)
		s.Params.ACLManager, err = aclstore.NewManager(ctx, aclstore.Params{
			Store:    s.ACLStore,
			RootPath: "/admin/acls",
			Authenticate: func(ctx context.Context, w http.ResponseWriter, req *http.Request) (aclstore.Identity, error) {
				id, err := s.Params.Authenticator.AuthenticateRequest(ctx, req)
				if err == nil {
					return id, nil
				}
				status, body := jemerror.Mapper(ctx, err)
				httprequest.WriteJSON(w, status, body)
				return nil, errgo.Mask(err, errgo.Any)
			},
			InitialAdminUsers: []string{string(jem.ControllerAdmin())},
		})
		c.Assert(err, gc.Equals, nil)
		s.AddCleanup(func(c *gc.C) {
			s.Params.ACLManager = nil
		})
	}

	if s.Params.ControllerAdmin == "" {
		s.Params.ControllerAdmin = jem.ControllerAdmin()
		s.AddCleanup(func(c *gc.C) {
			s.Params.ControllerAdmin = ""
		})
	}

	if s.Params.ControllerUUID == "" {
		s.Params.ControllerUUID = ControllerUUID
		s.AddCleanup(func(c *gc.C) {
			s.Params.ControllerUUID = ""
		})
	}

	s.APIHandler = s.NewAPIHTTPHandler(c, s.Params)
	if s.UseTLS {
		s.HTTP = httptest.NewTLSServer(s.APIHandler)
	} else {
		s.HTTP = httptest.NewServer(s.APIHandler)
	}
}

func (s *APISuite) TearDownTest(c *gc.C) {
	if s.HTTP != nil {
		s.HTTP.Close()
	}
	s.APIHandler = nil
	s.Candid.RemoveUsers()
	s.CleanupSuite.TearDownTest(c)
}

func (s *APISuite) NewAPIHTTPHandler(c *gc.C, p jemserver.HandlerParams) http.Handler {
	handlers, err := s.NewAPIHandler(context.Background(), p)
	c.Assert(err, gc.Equals, nil)

	r := httprouter.New()
	for _, h := range handlers {
		r.Handle(h.Method, h.Path, h.Handle)
	}
	return r
}

// Client returns an httpbakery client suitable for use when creating a
// juju API connection.
func (s *APISuite) Client(username string) *httpbakeryv3.Client {
	cl := s.Candid.Client(username)

	var cl3 httpbakeryv3.Client
	cl3.Client = cl.Client
	cl3.Logger = cl.Logger
	if cl.Key != nil {
		var kp bakeryv3.KeyPair
		kp.Public.Key = bakeryv3.Key(cl.Key.Public.Key)
		kp.Private.Key = bakeryv3.Key(cl.Key.Private.Key)
		cl3.Key = &kp

		agentv3.SetUpAuth(&cl3, &agentv3.AuthInfo{
			Key: cl3.Key,
			Agents: []agentv3.Agent{{
				URL:      s.Candid.URL.String(),
				Username: username,
			}},
		})
	}
	return &cl3
}

type bakeryLogger struct {
	c *gc.C
}

func (l bakeryLogger) Infof(_ context.Context, f string, args ...interface{}) {
	l.c.Logf(f, args...)
}

func (l bakeryLogger) Debugf(_ context.Context, f string, args ...interface{}) {
	l.c.Logf(f, args...)
}

type BootstrapAPISuite struct {
	jemtest.BootstrapSuite
	APISuite
}

func (s *BootstrapAPISuite) SetUpSuite(c *gc.C) {
	s.BootstrapSuite.SetUpSuite(c)
	s.APISuite.SetUpSuite(c)
}

func (s *BootstrapAPISuite) TearDownSuite(c *gc.C) {
	s.APISuite.TearDownSuite(c)
	s.BootstrapSuite.TearDownSuite(c)
}

func (s *BootstrapAPISuite) SetUpTest(c *gc.C) {
	s.APISuite.ConfigureController(&s.JujuConnSuite)
	s.BootstrapSuite.SetUpTest(c)
	s.APISuite.Params.JEMPool = s.Pool
	s.APISuite.Params.SessionPool = s.SessionPool
	s.APISuite.SetUpTest(c)
	conn := s.OpenControllerAPI(c)
	defer conn.Close()
	err := controllerapi.NewClient(conn).GrantController("everyone@external", "login")
	c.Assert(err, gc.Equals, nil)
}

func (s *BootstrapAPISuite) TearDownTest(c *gc.C) {
	s.APISuite.TearDownTest(c)
	s.BootstrapSuite.TearDownTest(c)
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
	return client.Do
}

// AnyBody is a convenience value that can be used in
// httptesting.AssertJSONCall.ExpectBody to cause
// AssertJSONCall to ignore the contents of the response body.
var AnyBody = httptesting.BodyAsserter(func(*gc.C, json.RawMessage) {})
