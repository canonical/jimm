package v1_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	gc "gopkg.in/check.v1"
	"gopkg.in/errgo.v1"

	"github.com/CanonicalLtd/jem/internal/jem"
	"github.com/CanonicalLtd/jem/internal/v1"
	"github.com/CanonicalLtd/jem/params"
	jujutesting "github.com/juju/testing"
	"github.com/juju/testing/httptesting"
	"gopkg.in/macaroon-bakery.v0/bakery"
	"gopkg.in/macaroon-bakery.v0/bakery/checkers"
	"gopkg.in/macaroon-bakery.v0/bakerytest"
	"gopkg.in/macaroon-bakery.v0/httpbakery"
	"gopkg.in/mgo.v2"
)

type APISuite struct {
	jujutesting.IsolatedMgoSuite
	srv        http.Handler
	jem        *jem.Pool
	discharger *bakerytest.Discharger
	username   string
	groups     []string
	client     *httpbakery.Client
}

var _ = gc.Suite(&APISuite{})

func (s *APISuite) SetUpTest(c *gc.C) {
	s.IsolatedMgoSuite.SetUpTest(c)
	s.srv, s.jem, s.discharger = s.newServer(c, s.Session)
	s.username = "testuser"
	s.client = httpbakery.NewClient()
}

func (s *APISuite) TearDownTest(c *gc.C) {
	s.discharger.Close()
	s.IsolatedMgoSuite.TearDownTest(c)
}

const adminUser = "admin"

func (s *APISuite) newServer(c *gc.C, session *mgo.Session) (http.Handler, *jem.Pool, *bakerytest.Discharger) {
	discharger := bakerytest.NewDischarger(nil, func(_ *http.Request, cond string, arg string) ([]checkers.Caveat, error) {
		if s.username == "" {
			return nil, errgo.Newf("no specified username for discharge macaroon")
		}
		return []checkers.Caveat{
			checkers.DeclaredCaveat(v1.UsernameAttr, s.username),
			checkers.DeclaredCaveat(v1.GroupsAttr, strings.Join(s.groups, " ")),
		}, nil
	})
	db := session.DB("jem")
	j, err := jem.NewPool(db, &bakery.NewServiceParams{})
	c.Assert(err, gc.IsNil)
	config := jem.ServerParams{
		DB:               db,
		StateServerAdmin: adminUser,
		IdentityLocation: discharger.Location(),
		PublicKeyLocator: discharger,
	}
	srv, err := jem.NewServer(config, map[string]jem.NewAPIHandlerFunc{"v1": v1.NewAPIHandler})
	c.Assert(err, gc.IsNil)
	return srv, j, discharger
}

func (s *APISuite) do(req *http.Request) (*http.Response, error) {
	if req.Body == nil {
		return s.client.Do(req)
	}
	return s.client.DoWithBody(req, req.Body.(io.ReadSeeker))
}

var addJESTests = []struct {
	about        string
	username     string
	body         interface{}
	expectStatus int
	expectBody   interface{}
}{{
	about: "add environment",
	body: params.ServerInfo{
		HostPorts:   []string{"0.1.2.3:1234"},
		CACert:      "dodgy",
		User:        "env-admin",
		Password:    "password",
		EnvironUUID: "deadbeef-dead-beef-dead-deaddeaddead",
	},
}, {
	about:    "incorrect user",
	username: "notadmin",
	body: params.ServerInfo{
		HostPorts:   []string{"0.1.2.3:1234"},
		CACert:      "dodgy",
		User:        "env-admin",
		Password:    "password",
		EnvironUUID: "deadbeef-dead-beef-dead-deaddeaddead",
	},
	expectStatus: http.StatusUnauthorized,
	expectBody: params.Error{
		Code:    "unauthorized",
		Message: "unauthorized",
	},
}, {
	about: "no hosts",
	body: params.ServerInfo{
		CACert:      "dodgy",
		User:        "env-admin",
		Password:    "password",
		EnvironUUID: "deadbeef-dead-beef-dead-deaddeaddead",
	},
	expectStatus: http.StatusBadRequest,
	expectBody: params.Error{
		Code:    "bad request",
		Message: "no host-ports in request",
	},
}, {
	about: "no ca-cert",
	body: params.ServerInfo{
		HostPorts:   []string{"0.1.2.3:1234"},
		User:        "env-admin",
		Password:    "password",
		EnvironUUID: "deadbeef-dead-beef-dead-deaddeaddead",
	},
	expectStatus: http.StatusBadRequest,
	expectBody: params.Error{
		Code:    "bad request",
		Message: "no ca-cert in request",
	},
}, {
	about: "no user",
	body: params.ServerInfo{
		HostPorts:   []string{"0.1.2.3:1234"},
		CACert:      "dodgy",
		Password:    "password",
		EnvironUUID: "deadbeef-dead-beef-dead-deaddeaddead",
	},
	expectStatus: http.StatusBadRequest,
	expectBody: params.Error{
		Code:    "bad request",
		Message: "no user in request",
	},
}, {
	about: "no environ uuid",
	body: params.ServerInfo{
		HostPorts: []string{"0.1.2.3:1234"},
		CACert:    "dodgy",
		User:      "env-admin",
		Password:  "password",
	},
	expectStatus: http.StatusBadRequest,
	expectBody: params.Error{
		Code:    "bad request",
		Message: "bad environment UUID in request",
	},
}}

func (s *APISuite) TestAddJES(c *gc.C) {
	s.username = adminUser
	for i, test := range addJESTests {
		c.Logf("%d: %s", i, test.about)
		username := test.username
		if username == "" {
			username = adminUser
		}
		httptesting.AssertJSONCall(c, httptesting.JSONCallParams{
			Method:       "PUT",
			Handler:      s.srv,
			JSONBody:     test.body,
			URL:          fmt.Sprintf("/v1/u/%s/server/env%d", username, i),
			Do:           s.do,
			ExpectStatus: test.expectStatus,
			ExpectBody:   test.expectBody,
		})
	}
}

func (s *APISuite) TestAddJESDuplicate(c *gc.C) {
	s.username = adminUser
	s.addJES(c, adminUser, "dupenv", &params.ServerInfo{
		HostPorts:   []string{"0.1.2.3:1234"},
		CACert:      "dodgy",
		User:        "env-admin",
		Password:    "password",
		EnvironUUID: "deadbeef-dead-beef-dead-deaddeaddead",
	})
	httptesting.AssertJSONCall(c, httptesting.JSONCallParams{
		Method:  "PUT",
		Handler: s.srv,
		URL:     "/v1/u/" + adminUser + "/server/dupenv",
		JSONBody: &params.ServerInfo{
			HostPorts:   []string{"0.1.2.3:1234"},
			CACert:      "dodgy",
			User:        "env-admin",
			Password:    "password",
			EnvironUUID: "deadbeef-dead-beef-dead-deaddeaddead",
		},
		ExpectBody: &params.Error{
			Message: "already exists",
			Code:    "already exists",
		},
		ExpectStatus: http.StatusForbidden,
		Do:           s.do,
	})
}

func (s *APISuite) addJES(c *gc.C, user, name string, jes *params.ServerInfo) {
	httptesting.AssertJSONCall(c, httptesting.JSONCallParams{
		Method:   "PUT",
		Handler:  s.srv,
		URL:      "/v1/u/" + user + "/server/" + name,
		JSONBody: jes,
		Do:       s.do,
	})
}

func (s *APISuite) TestAddJESUnauthenticated(c *gc.C) {
	httptesting.AssertJSONCall(c, httptesting.JSONCallParams{
		Method:  "PUT",
		Handler: s.srv,
		URL:     "/v1/u/user/server/env",
		ExpectBody: httptesting.BodyAsserter(func(c *gc.C, m json.RawMessage) {
			// Allow any body - the next check will check that it's a valid macaroon.
		}),
		ExpectStatus: http.StatusProxyAuthRequired,
	})
}
