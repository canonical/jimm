package v1_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/juju/juju/api"
	jujufeature "github.com/juju/juju/feature"
	corejujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/names"
	"github.com/juju/testing/httptesting"
	"github.com/juju/utils/featureflag"
	gc "gopkg.in/check.v1"
	"gopkg.in/errgo.v1"
	"gopkg.in/macaroon-bakery.v1/bakery/checkers"
	"gopkg.in/macaroon-bakery.v1/bakerytest"
	"gopkg.in/macaroon-bakery.v1/httpbakery"
	"gopkg.in/mgo.v2"

	"github.com/CanonicalLtd/jem/internal/jem"
	"github.com/CanonicalLtd/jem/internal/v1"
	"github.com/CanonicalLtd/jem/params"
)

type APISuite struct {
	corejujutesting.JujuConnSuite
	srv        *jem.Server
	discharger *bakerytest.Discharger
	username   string
	groups     []string
}

var _ = gc.Suite(&APISuite{})

func (s *APISuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	s.PatchValue(&jem.APIOpenTimeout, time.Duration(0))
	s.srv, s.discharger = s.newServer(c, s.Session)
	s.username = "testuser"
	os.Setenv("JUJU_DEV_FEATURE_FLAGS", jujufeature.JES)
	featureflag.SetFlagsFromEnvironment("JUJU_DEV_FEATURE_FLAGS")
}

func (s *APISuite) TearDownTest(c *gc.C) {
	s.discharger.Close()
	s.srv.Close()
	s.JujuConnSuite.TearDownTest(c)
}

const sshKey = "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQDOjaOjVRHchF2RFCKQdgBqrIA5nOoqSprLK47l2th5I675jw+QYMIihXQaITss3hjrh3+5ITyBO41PS5rHLNGtlYUHX78p9CHNZsJqHl/z1Ub1tuMe+/5SY2MkDYzgfPtQtVsLasAIiht/5g78AMMXH3HeCKb9V9cP6/lPPq6mCMvg8TDLrPp/P2vlyukAsJYUvVgoaPDUBpedHbkMj07pDJqe4D7c0yEJ8hQo/6nS+3bh9Q1NvmVNsB1pbtk3RKONIiTAXYcjclmOljxxJnl1O50F5sOIi38vyl7Q63f6a3bXMvJEf1lnPNJKAxspIfEu8gRasny3FEsbHfrxEwVj rog@rog-x220"

var dummyEnvConfig = map[string]interface{}{
	"authorized-keys": sshKey,
	"state-server":    true,
}

const adminUser = "admin"

func (s *APISuite) newServer(c *gc.C, session *mgo.Session) (*jem.Server, *bakerytest.Discharger) {
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
	config := jem.ServerParams{
		DB:               db,
		StateServerAdmin: adminUser,
		IdentityLocation: discharger.Location(),
		PublicKeyLocator: discharger,
	}
	srv, err := jem.NewServer(config, map[string]jem.NewAPIHandlerFunc{"v1": v1.NewAPIHandler})
	c.Assert(err, gc.IsNil)
	return srv, discharger
}

func (s *APISuite) TestAddJES(c *gc.C) {
	s.username = adminUser
	info := s.APIInfo(c)
	var addJESTests = []struct {
		about        string
		username     string
		body         params.ServerInfo
		expectStatus int
		expectBody   interface{}
	}{{
		about: "add environment",
		body: params.ServerInfo{
			HostPorts:   info.Addrs,
			CACert:      info.CACert,
			User:        info.Tag.Id(),
			Password:    info.Password,
			EnvironUUID: info.EnvironTag.Id(),
		},
	}, {
		about:    "incorrect user",
		username: "notadmin",
		body: params.ServerInfo{
			HostPorts:   info.Addrs,
			CACert:      info.CACert,
			User:        info.Tag.Id(),
			Password:    info.Password,
			EnvironUUID: info.EnvironTag.Id(),
		},
		expectStatus: http.StatusUnauthorized,
		expectBody: params.Error{
			Code:    "unauthorized",
			Message: "unauthorized",
		},
	}, {
		about: "no hosts",
		body: params.ServerInfo{
			CACert:      info.CACert,
			User:        info.Tag.Id(),
			Password:    info.Password,
			EnvironUUID: info.EnvironTag.Id(),
		},
		expectStatus: http.StatusBadRequest,
		expectBody: params.Error{
			Code:    "bad request",
			Message: "no host-ports in request",
		},
	}, {
		about: "no ca-cert",
		body: params.ServerInfo{
			HostPorts:   info.Addrs,
			User:        info.Tag.Id(),
			Password:    info.Password,
			EnvironUUID: info.EnvironTag.Id(),
		},
		expectStatus: http.StatusBadRequest,
		expectBody: params.Error{
			Code:    "bad request",
			Message: "no ca-cert in request",
		},
	}, {
		about: "no user",
		body: params.ServerInfo{
			HostPorts:   info.Addrs,
			CACert:      info.CACert,
			Password:    info.Password,
			EnvironUUID: info.EnvironTag.Id(),
		},
		expectStatus: http.StatusBadRequest,
		expectBody: params.Error{
			Code:    "bad request",
			Message: "no user in request",
		},
	}, {
		about: "no environ uuid",
		body: params.ServerInfo{
			HostPorts: info.Addrs,
			CACert:    info.CACert,
			User:      info.Tag.Id(),
			Password:  info.Password,
		},
		expectStatus: http.StatusBadRequest,
		expectBody: params.Error{
			Code:    "bad request",
			Message: "bad environment UUID in request",
		},
	}, {
		about: "cannot connect to evironment",
		body: params.ServerInfo{
			HostPorts:   []string{"0.1.2.3:1234"},
			CACert:      info.CACert,
			User:        info.Tag.Id(),
			Password:    info.Password,
			EnvironUUID: info.EnvironTag.Id(),
		},
		expectStatus: http.StatusBadRequest,
		expectBody: httptesting.BodyAsserter(func(c *gc.C, m json.RawMessage) {
			var body params.Error
			err := json.Unmarshal(m, &body)
			c.Assert(err, gc.IsNil)
			c.Assert(body.Code, gc.Equals, params.ErrBadRequest)
			c.Assert(body.Message, gc.Matches, `cannot connect to environment: unable to connect to ".*"`)
		}),
	}}
	for i, test := range addJESTests {
		c.Logf("test %d: %s", i, test.about)
		username := test.username
		if username == "" {
			username = adminUser
		}
		envname := fmt.Sprintf("env%d", i)
		httptesting.AssertJSONCall(c, httptesting.JSONCallParams{
			Method:       "PUT",
			Handler:      s.srv,
			JSONBody:     test.body,
			URL:          fmt.Sprintf("/v1/u/%s/server/%s", username, envname),
			Do:           bakeryDo(nil),
			ExpectStatus: test.expectStatus,
			ExpectBody:   test.expectBody,
		})
		if test.expectStatus != 0 {
			continue
		}
		// The server was added successfully. Check that we
		// can fetch its associated environment
		httptesting.AssertJSONCall(c, httptesting.JSONCallParams{
			Method:  "GET",
			Handler: s.srv,
			URL:     fmt.Sprintf("/v1/u/%s/env/%s", username, envname),
			ExpectBody: params.EnvironmentResponse{
				HostPorts: test.body.HostPorts,
				CACert:    test.body.CACert,
				UUID:      test.body.EnvironUUID,
			},
			Do: bakeryDo(nil),
		})
		// Clear the connection pool for the next test.
		s.srv.Pool().ClearAPIConnCache()
	}
}

func (s *APISuite) TestAddJESDuplicate(c *gc.C) {
	s.username = adminUser
	info := s.APIInfo(c)
	si := &params.ServerInfo{
		HostPorts:   info.Addrs,
		CACert:      info.CACert,
		User:        info.Tag.Id(),
		Password:    info.Password,
		EnvironUUID: info.EnvironTag.Id(),
	}
	s.addJES(c, adminUser, "dupenv", si)
	httptesting.AssertJSONCall(c, httptesting.JSONCallParams{
		Method:   "PUT",
		Handler:  s.srv,
		URL:      "/v1/u/" + adminUser + "/server/dupenv",
		JSONBody: si,
		ExpectBody: &params.Error{
			Message: "already exists",
			Code:    "already exists",
		},
		ExpectStatus: http.StatusForbidden,
		Do:           bakeryDo(nil),
	})
}

func (s *APISuite) addJES(c *gc.C, user, name string, jes *params.ServerInfo) {
	httptesting.AssertJSONCall(c, httptesting.JSONCallParams{
		Method:   "PUT",
		Handler:  s.srv,
		URL:      "/v1/u/" + user + "/server/" + name,
		JSONBody: jes,
		Do:       bakeryDo(nil),
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

func (s *APISuite) TestGetEnvironmentNotFound(c *gc.C) {
	httptesting.AssertJSONCall(c, httptesting.JSONCallParams{
		Method:  "GET",
		Handler: s.srv,
		URL:     "/v1/u/user/env/foo",
		ExpectBody: &params.Error{
			Message: `environment "user/foo" not found`,
			Code:    params.ErrNotFound,
		},
		ExpectStatus: http.StatusNotFound,
		Do:           bakeryDo(nil),
	})
}

func (s *APISuite) TestGetStateServer(c *gc.C) {
	srvId := s.addStateServer(c, adminUser, "foo")

	s.username = "bob"
	resp := httptesting.DoRequest(c, httptesting.DoRequestParams{
		Handler: s.srv,
		URL:     "/v1/u/" + srvId,
		Do:      bakeryDo(nil),
	})
	c.Assert(resp.Code, gc.Equals, http.StatusOK, gc.Commentf("body: %s", resp.Body.Bytes()))
	var jesInfo params.JESInfo
	err := json.Unmarshal(resp.Body.Bytes(), &jesInfo)
	c.Assert(err, gc.IsNil, gc.Commentf("body: %s", resp.Body.String()))
	c.Assert(jesInfo.ProviderType, gc.Equals, "dummy")
	c.Assert(jesInfo.Template, gc.Not(gc.HasLen), 0)
	c.Logf("%#v", jesInfo.Template)
}

func (s *APISuite) TestGetStateServerNotFound(c *gc.C) {
	httptesting.AssertJSONCall(c, httptesting.JSONCallParams{
		Method:  "GET",
		Handler: s.srv,
		URL:     "/v1/u/user/server/foo",
		ExpectBody: &params.Error{
			Message: `cannot open API: cannot get environment: environment "user/foo" not found`,
			Code:    params.ErrNotFound,
		},
		ExpectStatus: http.StatusNotFound,
		Do:           bakeryDo(nil),
	})
}

func (s *APISuite) TestNewEnvironment(c *gc.C) {
	srvId := s.addStateServer(c, adminUser, "foo")

	s.username = "bob"
	var envRespBody json.RawMessage
	httptesting.AssertJSONCall(c, httptesting.JSONCallParams{
		Method:  "POST",
		URL:     "/v1/u/bob/env",
		Handler: s.srv,
		JSONBody: params.NewEnvironmentInfo{
			Name:        params.Name("bar"),
			StateServer: srvId,
			Config:      dummyEnvConfig,
			Password:    "secret",
		},
		ExpectBody: httptesting.BodyAsserter(func(_ *gc.C, body json.RawMessage) {
			envRespBody = body
		}),
		Do: bakeryDo(nil),
	})
	var envResp params.EnvironmentResponse
	err := json.Unmarshal(envRespBody, &envResp)
	c.Assert(err, gc.IsNil)

	// Ensure that we can connect to the new environment
	apiInfo := &api.Info{
		Tag:        names.NewUserTag("bob"),
		Password:   "secret",
		Addrs:      envResp.HostPorts,
		CACert:     envResp.CACert,
		EnvironTag: names.NewEnvironTag(envResp.UUID),
	}
	st, err := api.Open(apiInfo, api.DialOpts{})
	c.Assert(err, gc.IsNil)
	defer st.Close()
}

var newEnvironmentWithInvalidStateServerPathTests = []struct {
	path      string
	expectErr string
}{{
	path:      "x",
	expectErr: `wrong number of parts`,
}, {
	path:      "x/foo/y",
	expectErr: `second part of state server id must be "server"`,
}, {
	path:      "/server/foo",
	expectErr: `empty user name or entity name`,
}, {
	path:      "foo/server/",
	expectErr: `empty user name or entity name`,
}}

func (s *APISuite) TestNewEnvironmentWithInvalidStateServerPath(c *gc.C) {
	s.username = "bob"
	for _, test := range newEnvironmentWithInvalidStateServerPathTests {
		httptesting.AssertJSONCall(c, httptesting.JSONCallParams{
			Method:  "POST",
			URL:     "/v1/u/bob/env",
			Handler: s.srv,
			JSONBody: params.NewEnvironmentInfo{
				Name:        params.Name("bar"),
				StateServer: test.path,
			},
			ExpectBody: params.Error{
				Message: fmt.Sprintf("cannot parse state server path %q: %s", test.path, test.expectErr),
				Code:    params.ErrBadRequest,
			},
			ExpectStatus: http.StatusBadRequest,
			Do:           bakeryDo(nil),
		})
	}
}

func (s *APISuite) TestNewEnvironmentCannotOpenAPI(c *gc.C) {
	s.username = "bob"
	httptesting.AssertJSONCall(c, httptesting.JSONCallParams{
		Method:  "POST",
		URL:     "/v1/u/bob/env",
		Handler: s.srv,
		JSONBody: params.NewEnvironmentInfo{
			Name:        params.Name("bar"),
			StateServer: "bob/server/foo",
		},
		ExpectBody: params.Error{
			Message: `cannot connect to state server: cannot get environment: environment "bob/foo" not found`,
			Code:    params.ErrNotFound,
		},
		ExpectStatus: http.StatusNotFound,
		Do:           bakeryDo(nil),
	})
}

func (s *APISuite) TestNewEnvironmentInvalidConfig(c *gc.C) {
	srvId := s.addStateServer(c, adminUser, "foo")
	s.username = "bob"

	httptesting.AssertJSONCall(c, httptesting.JSONCallParams{
		Method:  "POST",
		URL:     "/v1/u/bob/env",
		Handler: s.srv,
		JSONBody: params.NewEnvironmentInfo{
			Name:        params.Name("bar"),
			StateServer: srvId,
			Config: map[string]interface{}{
				"authorized-keys": 123,
			},
		},
		ExpectBody: params.Error{
			Message: `cannot validate attributes: authorized-keys: expected string, got float64(123)`,
			Code:    params.ErrBadRequest,
		},
		ExpectStatus: http.StatusBadRequest,
		Do:           bakeryDo(nil),
	})
}

func (s *APISuite) TestNewEnvironmentTwice(c *gc.C) {
	srvId := s.addStateServer(c, adminUser, "foo")
	s.username = "bob"

	body := &params.NewEnvironmentInfo{
		Name:        "bar",
		Password:    "password",
		StateServer: srvId,
		Config:      dummyEnvConfig,
	}
	p := httptesting.JSONCallParams{
		Method:     "POST",
		URL:        "/v1/u/bob/env",
		Handler:    s.srv,
		JSONBody:   body,
		ExpectBody: anyBody,
		Do:         bakeryDo(nil),
	}
	httptesting.AssertJSONCall(c, p)

	// Creating the environment the second time may fail because
	// the juju user does not need to be created the second time.
	// This test ensures that this works OK.
	body.Name = "bar2"
	httptesting.AssertJSONCall(c, p)

	// Check that if we use the same name again, we get an error.
	p.ExpectBody = params.Error{
		Code:    params.ErrAlreadyExists,
		Message: "already exists",
	}
	p.ExpectStatus = http.StatusForbidden
	httptesting.AssertJSONCall(c, p)
}

func (s *APISuite) TestNewEnvironmentWithNoPassword(c *gc.C) {
	srvId := s.addStateServer(c, adminUser, "foo")
	s.username = "bob"

	// N.B. "state-server" is a required attribute
	httptesting.AssertJSONCall(c, httptesting.JSONCallParams{
		Method:  "POST",
		URL:     "/v1/u/bob/env",
		Handler: s.srv,
		JSONBody: params.NewEnvironmentInfo{
			Name:        "bar",
			StateServer: srvId,
			Config: map[string]interface{}{
				"authorized-keys": sshKey,
			},
		},
		ExpectBody: params.Error{
			Code:    params.ErrBadRequest,
			Message: `cannot create user: no password specified for new user`,
		},
		ExpectStatus: http.StatusBadRequest,
		Do:           bakeryDo(nil),
	})
}

func (s *APISuite) TestNewEnvironmentCannotCreate(c *gc.C) {
	srvId := s.addStateServer(c, adminUser, "foo")
	s.username = "bob"

	// N.B. "state-server" is a required attribute
	httptesting.AssertJSONCall(c, httptesting.JSONCallParams{
		Method:  "POST",
		URL:     "/v1/u/bob/env",
		Handler: s.srv,
		JSONBody: params.NewEnvironmentInfo{
			Name:        "bar",
			Password:    "secret",
			StateServer: srvId,
			Config: map[string]interface{}{
				"authorized-keys": sshKey,
			},
		},
		ExpectBody: params.Error{
			Message: `cannot create environment: provider validation failed: state-server: expected bool, got nothing`,
		},
		ExpectStatus: http.StatusInternalServerError,
		Do:           bakeryDo(nil),
	})

	// Check that the environment is not there (it was added temporarily during the call).
	httptesting.AssertJSONCall(c, httptesting.JSONCallParams{
		Method:  "GET",
		Handler: s.srv,
		URL:     "/v1/u/bob/env/bar",
		ExpectBody: &params.Error{
			Message: `environment "bob/bar" not found`,
			Code:    params.ErrNotFound,
		},
		ExpectStatus: http.StatusNotFound,
		Do:           bakeryDo(nil),
	})
}

func (s *APISuite) TestNewEnvironmentUnauthorized(c *gc.C) {
	srvId := s.addStateServer(c, adminUser, "foo")
	s.username = "charlie"

	httptesting.AssertJSONCall(c, httptesting.JSONCallParams{
		Method:  "POST",
		URL:     "/v1/u/bob/env",
		Handler: s.srv,
		JSONBody: params.NewEnvironmentInfo{
			Name:        "bar",
			StateServer: srvId,
			Config:      dummyEnvConfig,
		},
		ExpectBody: params.Error{
			Message: `unauthorized`,
			Code:    params.ErrUnauthorized,
		},
		ExpectStatus: http.StatusUnauthorized,
		Do:           bakeryDo(nil),
	})
}

// addStateServer adds a new stateserver named name under the
// given user. It returns the state server id.
func (s *APISuite) addStateServer(c *gc.C, user, name string) string {
	// Note that because the cookies acquired in this request don't
	// persist, the discharge macaroon we get won't affect subsequent
	// requests in the caller.
	olduser := s.username
	defer func() {
		s.username = olduser
	}()
	s.username = adminUser

	info := s.APIInfo(c)

	// First add the state server that we'll use to create the environment.
	srvId := adminUser + "/server/foo"
	c.Logf("user: %v", info.Tag.Id())
	httptesting.AssertJSONCall(c, httptesting.JSONCallParams{
		Method:  "PUT",
		Handler: s.srv,
		JSONBody: params.ServerInfo{
			HostPorts:   info.Addrs,
			CACert:      info.CACert,
			User:        info.Tag.Id(),
			Password:    info.Password,
			EnvironUUID: info.EnvironTag.Id(),
		},
		URL: "/v1/u/" + srvId,
		Do:  bakeryDo(nil),
	})
	return srvId
}

func bakeryDo(client *http.Client) func(*http.Request) (*http.Response, error) {
	if client == nil {
		client = httpbakery.NewHTTPClient()
	}
	bclient := httpbakery.NewClient()
	bclient.Client = client
	return func(req *http.Request) (*http.Response, error) {
		if req.Body != nil {
			body := req.Body.(io.ReadSeeker)
			req.Body = nil
			return bclient.DoWithBody(req, body)
		}
		return bclient.Do(req)
	}
}

var anyBody = httptesting.BodyAsserter(func(*gc.C, json.RawMessage) {
})
