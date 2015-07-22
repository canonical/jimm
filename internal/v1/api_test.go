package v1_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"time"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/usermanager"
	jujufeature "github.com/juju/juju/feature"
	corejujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/testing/httptesting"
	"github.com/juju/utils/featureflag"
	gc "gopkg.in/check.v1"
	"gopkg.in/macaroon-bakery.v1/httpbakery"
	"gopkg.in/mgo.v2"

	"github.com/CanonicalLtd/jem/internal/idmtest"
	"github.com/CanonicalLtd/jem/internal/jem"
	"github.com/CanonicalLtd/jem/internal/v1"
	"github.com/CanonicalLtd/jem/jemclient"
	"github.com/CanonicalLtd/jem/params"
)

type APISuite struct {
	corejujutesting.JujuConnSuite
	srv     *jem.Server
	httpSrv *httptest.Server
	idmSrv  *idmtest.Server
}

var _ = gc.Suite(&APISuite{})

func (s *APISuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	s.PatchValue(&jem.APIOpenTimeout, time.Duration(0))
	s.idmSrv = idmtest.NewServer()
	s.srv = s.newServer(c, s.Session, s.idmSrv)
	os.Setenv("JUJU_DEV_FEATURE_FLAGS", jujufeature.JES)
	featureflag.SetFlagsFromEnvironment("JUJU_DEV_FEATURE_FLAGS")
	s.httpSrv = httptest.NewServer(s.srv)
}

func (s *APISuite) client(username params.User) *jemclient.Client {
	return jemclient.New(jemclient.NewParams{
		BaseURL: s.httpSrv.URL,
		Client:  s.idmSrv.Client(string(username)),
	})
}

func (s *APISuite) TearDownTest(c *gc.C) {
	s.srv.Close()
	s.httpSrv.Close()
	s.idmSrv.Close()
	s.JujuConnSuite.TearDownTest(c)
}

const sshKey = "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQDOjaOjVRHchF2RFCKQdgBqrIA5nOoqSprLK47l2th5I675jw+QYMIihXQaITss3hjrh3+5ITyBO41PS5rHLNGtlYUHX78p9CHNZsJqHl/z1Ub1tuMe+/5SY2MkDYzgfPtQtVsLasAIiht/5g78AMMXH3HeCKb9V9cP6/lPPq6mCMvg8TDLrPp/P2vlyukAsJYUvVgoaPDUBpedHbkMj07pDJqe4D7c0yEJ8hQo/6nS+3bh9Q1NvmVNsB1pbtk3RKONIiTAXYcjclmOljxxJnl1O50F5sOIi38vyl7Q63f6a3bXMvJEf1lnPNJKAxspIfEu8gRasny3FEsbHfrxEwVj rog@rog-x220"

var dummyEnvConfig = map[string]interface{}{
	"authorized-keys": sshKey,
	"state-server":    true,
}

func (s *APISuite) newServer(c *gc.C, session *mgo.Session, idmSrv *idmtest.Server) *jem.Server {
	db := session.DB("jem")
	s.idmSrv.AddUser("agent")
	config := jem.ServerParams{
		DB:               db,
		StateServerAdmin: "admin",
		IdentityLocation: idmSrv.URL.String(),
		PublicKeyLocator: idmSrv,
		AgentUsername:    "agent",
		AgentKey:         s.idmSrv.UserPublicKey("agent"),
	}
	srv, err := jem.NewServer(config, map[string]jem.NewAPIHandlerFunc{"v1": v1.NewAPIHandler})
	c.Assert(err, gc.IsNil)
	return srv
}

var unauthorizedTests = []struct {
	about  string
	asUser string
	method string
	path   string
	body   interface{}
}{{
	about:  "get env as non-owner",
	asUser: "other",
	method: "GET",
	path:   "/v1/env/bob/private",
}, {
	about:  "get server as non-owner",
	asUser: "other",
	method: "GET",
	path:   "/v1/server/bob/private",
}, {
	about:  "new env as non-owner",
	asUser: "other",
	method: "POST",
	path:   "/v1/env/bob",
	body: params.NewEnvironmentInfo{
		Name:        "newenv",
		StateServer: params.EntityPath{"bob", "open"},
	},
}, {
	about:  "new env with inaccessible state server",
	asUser: "alice",
	method: "POST",
	path:   "/v1/env/alice",
	body: params.NewEnvironmentInfo{
		Name:        "newenv",
		StateServer: params.EntityPath{"bob", "private"},
	},
}, {
	about:  "set server perm as non-owner",
	asUser: "other",
	method: "PUT",
	path:   "/v1/server/bob/private/perm",
	body:   params.ACL{},
}, {
	about:  "set env perm as non-owner",
	asUser: "other",
	method: "PUT",
	path:   "/v1/env/bob/private/perm",
	body:   params.ACL{},
}, {
	about:  "get server perm as non-owner",
	asUser: "other",
	method: "GET",
	path:   "/v1/server/bob/private/perm",
}, {
	about:  "get env perm as non-owner",
	asUser: "other",
	method: "GET",
	path:   "/v1/env/bob/private/perm",
}, {
	about:  "get server perm with ACL that allows us",
	asUser: "other",
	method: "GET",
	path:   "/v1/server/bob/open/perm",
}, {
	about:  "get env perm with ACL that allows us",
	asUser: "other",
	method: "GET",
	path:   "/v1/env/bob/open/perm",
}}

func (s *APISuite) TestUnauthorized(c *gc.C) {
	s.addStateServer(c, params.EntityPath{"bob", "private"})

	s.addStateServer(c, params.EntityPath{"bob", "open"})
	s.allowServerAllPerm(c, params.EntityPath{"bob", "open"})
	s.allowEnvAllPerm(c, params.EntityPath{"bob", "open"})

	for i, test := range unauthorizedTests {
		c.Logf("test %d: %s", i, test.about)
		httptesting.AssertJSONCall(c, httptesting.JSONCallParams{
			Method:   test.method,
			Handler:  s.srv,
			JSONBody: test.body,
			URL:      test.path,
			ExpectBody: &params.Error{
				Message: `unauthorized`,
				Code:    params.ErrUnauthorized,
			},
			ExpectStatus: http.StatusUnauthorized,
			Do:           bakeryDo(s.idmSrv.Client(test.asUser)),
		})
	}
}

func (s *APISuite) TestAddJES(c *gc.C) {
	info := s.APIInfo(c)
	var addJESTests = []struct {
		about        string
		authUser     params.User
		username     params.User
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
		about:    "add environment as part of group",
		username: "beatles",
		authUser: "alice",
		body: params.ServerInfo{
			HostPorts:   info.Addrs,
			CACert:      info.CACert,
			User:        info.Tag.Id(),
			Password:    info.Password,
			EnvironUUID: info.EnvironTag.Id(),
		},
	}, {
		about:    "incorrect user",
		authUser: "alice",
		username: "bob",
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
	s.idmSrv.AddUser("alice", "beatles")
	s.idmSrv.AddUser("bob", "beatles")
	for i, test := range addJESTests {
		c.Logf("test %d: %s", i, test.about)
		envPath := params.EntityPath{
			User: test.username,
			Name: params.Name(fmt.Sprintf("env%d", i)),
		}
		if envPath.User == "" {
			envPath.User = "testuser"
		}
		authUser := test.authUser
		if authUser == "" {
			authUser = envPath.User
		}
		client := s.idmSrv.Client(string(authUser))
		httptesting.AssertJSONCall(c, httptesting.JSONCallParams{
			Method:       "PUT",
			Handler:      s.srv,
			JSONBody:     test.body,
			URL:          fmt.Sprintf("/v1/server/%s", envPath),
			Do:           bakeryDo(client),
			ExpectStatus: test.expectStatus,
			ExpectBody:   test.expectBody,
		})
		if test.expectStatus != 0 {
			continue
		}
		// The server was added successfully. Check that we
		// can fetch its associated environment and that we
		// can connect to that.
		envResp, err := s.client(authUser).GetEnvironment(&params.GetEnvironment{
			EntityPath: envPath,
		})
		c.Assert(err, gc.IsNil)
		c.Assert(envResp, jc.DeepEquals, &params.EnvironmentResponse{
			Path:      envPath,
			User:      test.body.User,
			HostPorts: test.body.HostPorts,
			CACert:    test.body.CACert,
			UUID:      test.body.EnvironUUID,
		})
		st := openAPIFromEnvironmentResponse(c, envResp, test.body.Password)
		st.Close()
		// Clear the connection pool for the next test.
		s.srv.Pool().ClearAPIConnCache()
	}
}

func (s *APISuite) TestAddJESDuplicate(c *gc.C) {
	info := s.APIInfo(c)
	si := &params.ServerInfo{
		HostPorts:   info.Addrs,
		CACert:      info.CACert,
		User:        info.Tag.Id(),
		Password:    info.Password,
		EnvironUUID: info.EnvironTag.Id(),
	}
	srvPath := params.EntityPath{"bob", "dupenv"}
	s.addJES(c, srvPath, si)
	httptesting.AssertJSONCall(c, httptesting.JSONCallParams{
		Method:   "PUT",
		Handler:  s.srv,
		URL:      "/v1/server/" + srvPath.String(),
		JSONBody: si,
		ExpectBody: &params.Error{
			Message: "already exists",
			Code:    "already exists",
		},
		ExpectStatus: http.StatusForbidden,
		Do:           bakeryDo(s.idmSrv.Client("bob")),
	})
}

func (s *APISuite) addJES(c *gc.C, path params.EntityPath, jes *params.ServerInfo) {
	httptesting.AssertJSONCall(c, httptesting.JSONCallParams{
		Method:   "PUT",
		Handler:  s.srv,
		URL:      "/v1/server/" + path.String(),
		JSONBody: jes,
		Do:       bakeryDo(s.idmSrv.Client(string(path.User))),
	})
}

func (s *APISuite) TestAddJESUnauthenticated(c *gc.C) {
	httptesting.AssertJSONCall(c, httptesting.JSONCallParams{
		Method:  "PUT",
		Handler: s.srv,
		URL:     "/v1/server/user/env",
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
		URL:     "/v1/env/user/foo",
		ExpectBody: &params.Error{
			Message: `environment "user/foo" not found`,
			Code:    params.ErrNotFound,
		},
		ExpectStatus: http.StatusNotFound,
		Do:           bakeryDo(s.idmSrv.Client("user")),
	})

	// If we're some different user, we get Unauthorized.
	httptesting.AssertJSONCall(c, httptesting.JSONCallParams{
		Method:  "GET",
		Handler: s.srv,
		URL:     "/v1/env/user/foo",
		ExpectBody: &params.Error{
			Message: `unauthorized`,
			Code:    params.ErrUnauthorized,
		},
		ExpectStatus: http.StatusUnauthorized,
		Do:           bakeryDo(s.idmSrv.Client("other")),
	})
}

func (s *APISuite) TestGetStateServer(c *gc.C) {
	srvId := s.addStateServer(c, params.EntityPath{"bob", "foo"})

	resp := httptesting.DoRequest(c, httptesting.DoRequestParams{
		Handler: s.srv,
		URL:     "/v1/server/" + srvId.String(),
		Do:      bakeryDo(s.idmSrv.Client("bob")),
	})
	c.Assert(resp.Code, gc.Equals, http.StatusOK, gc.Commentf("body: %s", resp.Body.Bytes()))
	var jesInfo params.JESResponse
	err := json.Unmarshal(resp.Body.Bytes(), &jesInfo)
	c.Assert(err, gc.IsNil, gc.Commentf("body: %s", resp.Body.String()))
	c.Assert(jesInfo.ProviderType, gc.Equals, "dummy")
	c.Assert(jesInfo.Template, gc.Not(gc.HasLen), 0)
	// Check that all path attributes have been removed.
	for name := range jesInfo.Template {
		c.Assert(strings.HasSuffix(name, "-path"), gc.Equals, false)
	}
	c.Logf("%#v", jesInfo.Template)
}

func (s *APISuite) TestGetStateServerNotFound(c *gc.C) {
	httptesting.AssertJSONCall(c, httptesting.JSONCallParams{
		Method:  "GET",
		Handler: s.srv,
		URL:     "/v1/server/bob/foo",
		ExpectBody: &params.Error{
			Message: `cannot open API: cannot get environment: environment "bob/foo" not found`,
			Code:    params.ErrNotFound,
		},
		ExpectStatus: http.StatusNotFound,
		Do:           bakeryDo(s.idmSrv.Client("bob")),
	})

	// Any other user just sees Unauthorized.
	httptesting.AssertJSONCall(c, httptesting.JSONCallParams{
		Method:  "GET",
		Handler: s.srv,
		URL:     "/v1/server/bob/foo",
		ExpectBody: &params.Error{
			Message: `unauthorized`,
			Code:    params.ErrUnauthorized,
		},
		ExpectStatus: http.StatusUnauthorized,
		Do:           bakeryDo(s.idmSrv.Client("alice")),
	})
}

func (s *APISuite) TestNewEnvironment(c *gc.C) {
	srvId := s.addStateServer(c, params.EntityPath{"bob", "foo"})

	var envRespBody json.RawMessage
	httptesting.AssertJSONCall(c, httptesting.JSONCallParams{
		Method:  "POST",
		URL:     "/v1/env/bob",
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
		Do: bakeryDo(s.idmSrv.Client("bob")),
	})
	var envResp params.EnvironmentResponse
	err := json.Unmarshal(envRespBody, &envResp)
	c.Assert(err, gc.IsNil)

	c.Assert(envResp.ServerUUID, gc.Equals, s.APIInfo(c).EnvironTag.Id())

	st := openAPIFromEnvironmentResponse(c, &envResp, "secret")
	st.Close()

	// Ensure that we can connect to the new environment
	// from the information returned by GetEnvironment.
	envResp2, err := s.client("bob").GetEnvironment(&params.GetEnvironment{
		EntityPath: params.EntityPath{
			User: "bob",
			Name: "bar",
		},
	})
	c.Assert(err, gc.IsNil)
	st = openAPIFromEnvironmentResponse(c, envResp2, "secret")
	st.Close()
}

func openAPIFromEnvironmentResponse(c *gc.C, resp *params.EnvironmentResponse, password string) *api.State {
	// Ensure that we can connect to the new environment
	apiInfo := &api.Info{
		Tag:        names.NewUserTag(resp.User),
		Password:   password,
		Addrs:      resp.HostPorts,
		CACert:     resp.CACert,
		EnvironTag: names.NewEnvironTag(resp.UUID),
	}
	st, err := api.Open(apiInfo, api.DialOpts{})
	c.Assert(err, gc.IsNil, gc.Commentf("user: %q; password: %q", resp.User, password))
	return st
}

func (s *APISuite) TestNewEnvironmentUnderGroup(c *gc.C) {
	srvId := s.addStateServer(c, params.EntityPath{"bob", "foo"})

	s.idmSrv.AddUser("bob", "beatles")
	var envRespBody json.RawMessage
	httptesting.AssertJSONCall(c, httptesting.JSONCallParams{
		Method:  "POST",
		URL:     "/v1/env/beatles",
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
		Do: bakeryDo(s.idmSrv.Client("bob")),
	})
	var envResp params.EnvironmentResponse
	err := json.Unmarshal(envRespBody, &envResp)
	c.Assert(err, gc.IsNil)

	c.Assert(envResp.ServerUUID, gc.Equals, s.APIInfo(c).EnvironTag.Id())

	// Ensure that we can connect to the new environment
	apiInfo := &api.Info{
		Tag:        names.NewUserTag(string(envResp.User)),
		Password:   "secret",
		Addrs:      envResp.HostPorts,
		CACert:     envResp.CACert,
		EnvironTag: names.NewEnvironTag(envResp.UUID),
	}
	st, err := api.Open(apiInfo, api.DialOpts{})
	c.Assert(err, gc.IsNil)
	defer st.Close()
}

func (s *APISuite) TestNewEnvironmentWithExistingUser(c *gc.C) {
	username := "jem-bob--bar"

	_, err := usermanager.NewClient(s.APIState).AddUser(username, "", "old")
	c.Assert(err, gc.IsNil)

	srvId := s.addStateServer(c, params.EntityPath{"bob", "foo"})

	var envRespBody json.RawMessage
	httptesting.AssertJSONCall(c, httptesting.JSONCallParams{
		Method:  "POST",
		URL:     "/v1/env/bob",
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
		Do: bakeryDo(s.idmSrv.Client("bob")),
	})
	var envResp params.EnvironmentResponse
	err = json.Unmarshal(envRespBody, &envResp)
	c.Assert(err, gc.IsNil)

	c.Assert(envResp.ServerUUID, gc.Equals, s.APIInfo(c).EnvironTag.Id())

	// Make sure that we really are reusing the username.
	c.Assert(envResp.User, gc.Equals, username)

	// Ensure that we can connect to the new environment with
	// the new secret
	apiInfo := &api.Info{
		Tag:        names.NewUserTag(username),
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
	expectErr: `wrong number of parts in entity path`,
}, {
	path:      "/foo",
	expectErr: `invalid user name ""`,
}, {
	path:      "foo/",
	expectErr: `invalid name ""`,
}}

func (s *APISuite) TestNewEnvironmentWithInvalidStateServerPath(c *gc.C) {
	for i, test := range newEnvironmentWithInvalidStateServerPathTests {
		c.Logf("test %d", i)
		httptesting.AssertJSONCall(c, httptesting.JSONCallParams{
			Method:  "POST",
			URL:     "/v1/env/bob",
			Handler: s.srv,
			JSONBody: map[string]interface{}{
				"name":         "bar",
				"state-server": test.path,
			},
			ExpectBody: params.Error{
				Message: fmt.Sprintf("cannot unmarshal parameters: cannot unmarshal into field: cannot unmarshal request body: %s", test.expectErr),
				Code:    params.ErrBadRequest,
			},
			ExpectStatus: http.StatusBadRequest,
			Do:           bakeryDo(s.idmSrv.Client("bob")),
		})
	}
}

func (s *APISuite) TestNewEnvironmentCannotOpenAPI(c *gc.C) {
	httptesting.AssertJSONCall(c, httptesting.JSONCallParams{
		Method:  "POST",
		URL:     "/v1/env/bob",
		Handler: s.srv,
		JSONBody: params.NewEnvironmentInfo{
			Name:        params.Name("bar"),
			StateServer: params.EntityPath{"bob", "foo"},
		},
		ExpectBody: params.Error{
			Message: `cannot connect to state server: cannot get environment: environment "bob/foo" not found`,
			Code:    params.ErrNotFound,
		},
		ExpectStatus: http.StatusNotFound,
		Do:           bakeryDo(s.idmSrv.Client("bob")),
	})
}

func (s *APISuite) TestNewEnvironmentInvalidConfig(c *gc.C) {
	srvId := s.addStateServer(c, params.EntityPath{"bob", "foo"})

	httptesting.AssertJSONCall(c, httptesting.JSONCallParams{
		Method:  "POST",
		URL:     "/v1/env/bob",
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
		Do:           bakeryDo(s.idmSrv.Client("bob")),
	})
}

func (s *APISuite) TestNewEnvironmentTwice(c *gc.C) {
	srvId := s.addStateServer(c, params.EntityPath{"bob", "foo"})

	body := &params.NewEnvironmentInfo{
		Name:        "bar",
		Password:    "password",
		StateServer: srvId,
		Config:      dummyEnvConfig,
	}
	p := httptesting.JSONCallParams{
		Method:     "POST",
		URL:        "/v1/env/bob",
		Handler:    s.srv,
		JSONBody:   body,
		ExpectBody: anyBody,
		Do:         bakeryDo(s.idmSrv.Client("bob")),
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
	srvId := s.addStateServer(c, params.EntityPath{"bob", "foo"})

	// N.B. "state-server" is a required attribute
	httptesting.AssertJSONCall(c, httptesting.JSONCallParams{
		Method:  "POST",
		URL:     "/v1/env/bob",
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
			Message: `cannot create user: no password specified`,
		},
		ExpectStatus: http.StatusBadRequest,
		Do:           bakeryDo(s.idmSrv.Client("bob")),
	})
}

func (s *APISuite) TestNewEnvironmentCannotCreate(c *gc.C) {
	srvId := s.addStateServer(c, params.EntityPath{"bob", "foo"})

	// N.B. "state-server" is a required attribute
	httptesting.AssertJSONCall(c, httptesting.JSONCallParams{
		Method:  "POST",
		URL:     "/v1/env/bob",
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
		Do:           bakeryDo(s.idmSrv.Client("bob")),
	})

	// Check that the environment is not there (it was added temporarily during the call).
	httptesting.AssertJSONCall(c, httptesting.JSONCallParams{
		Method:  "GET",
		Handler: s.srv,
		URL:     "/v1/env/bob/bar",
		ExpectBody: &params.Error{
			Message: `environment "bob/bar" not found`,
			Code:    params.ErrNotFound,
		},
		ExpectStatus: http.StatusNotFound,
		Do:           bakeryDo(s.idmSrv.Client("bob")),
	})
}

func (s *APISuite) TestNewEnvironmentUnauthorized(c *gc.C) {
	srvId := s.addStateServer(c, params.EntityPath{"bob", "foo"})

	httptesting.AssertJSONCall(c, httptesting.JSONCallParams{
		Method:  "POST",
		URL:     "/v1/env/bob",
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
		Do:           bakeryDo(s.idmSrv.Client("other")),
	})
}

func (s *APISuite) TestListJES(c *gc.C) {
	srvId := s.addStateServer(c, params.EntityPath{"bob", "foo"})
	resp, err := s.client("bob").ListJES(nil)
	c.Assert(err, gc.IsNil)
	c.Assert(resp, jc.DeepEquals, &params.ListJESResponse{
		StateServers: []params.JESResponse{{
			Path: srvId,
		}},
	})

	// Check that the entry doesn't show up when listing
	// as a different user.
	resp, err = s.client("alice").ListJES(nil)
	c.Assert(err, gc.IsNil)
	c.Assert(resp, jc.DeepEquals, &params.ListJESResponse{})
}

func (s *APISuite) TestListJESNoServers(c *gc.C) {
	resp, err := s.client("bob").ListJES(nil)
	c.Assert(err, gc.IsNil)
	c.Assert(resp, jc.DeepEquals, &params.ListJESResponse{})
}

func (s *APISuite) TestListEnvironmentsNoServers(c *gc.C) {
	resp, err := s.client("bob").ListEnvironments(nil)
	c.Assert(err, gc.IsNil)
	c.Assert(resp, jc.DeepEquals, &params.ListEnvironmentsResponse{})
}

func (s *APISuite) TestListEnvironmentsStateServerOnly(c *gc.C) {
	srvId := s.addStateServer(c, params.EntityPath{"bob", "foo"})
	info := s.APIInfo(c)
	resp, err := s.client("bob").ListEnvironments(nil)
	c.Assert(err, gc.IsNil)
	c.Assert(resp, jc.DeepEquals, &params.ListEnvironmentsResponse{
		Environments: []params.EnvironmentResponse{{
			Path:      srvId,
			User:      info.Tag.Id(),
			UUID:      info.EnvironTag.Id(),
			CACert:    info.CACert,
			HostPorts: info.Addrs,
		}},
	})
}

func (s *APISuite) allowServerAllPerm(c *gc.C, path params.EntityPath) {
	err := s.client(path.User).SetStateServerPerm(&params.SetStateServerPerm{
		EntityPath: path,
		ACL: params.ACL{
			Read: []string{"everyone"},
		},
	})
	c.Assert(err, gc.IsNil)
}

func (s *APISuite) allowEnvAllPerm(c *gc.C, path params.EntityPath) {
	err := s.client(path.User).SetEnvironmentPerm(&params.SetEnvironmentPerm{
		EntityPath: path,
		ACL: params.ACL{
			Read: []string{"everyone"},
		},
	})
	c.Assert(err, gc.IsNil)
}

func (s *APISuite) TestListEnvironments(c *gc.C) {
	srvId := s.addStateServer(c, params.EntityPath{"alice", "foo"})
	s.allowEnvAllPerm(c, srvId)
	s.allowServerAllPerm(c, srvId)
	envId1, user1, uuid1 := s.addEnvironment(c, srvId, params.EntityPath{"bob", "bar"})
	envId2, user2, uuid2 := s.addEnvironment(c, srvId, params.EntityPath{"charlie", "bar"})
	info := s.APIInfo(c)

	resps := []params.EnvironmentResponse{{
		Path:      srvId,
		User:      info.Tag.Id(),
		UUID:      info.EnvironTag.Id(),
		CACert:    info.CACert,
		HostPorts: info.Addrs,
	}, {
		Path:      envId1,
		User:      user1,
		UUID:      uuid1,
		CACert:    info.CACert,
		HostPorts: info.Addrs,
	}, {
		Path:      envId2,
		User:      user2,
		UUID:      uuid2,
		CACert:    info.CACert,
		HostPorts: info.Addrs,
	}}
	tests := []struct {
		user    params.User
		indexes []int
	}{{
		user:    "bob",
		indexes: []int{0, 1},
	}, {
		user:    "charlie",
		indexes: []int{0, 2},
	}, {
		user:    "alice",
		indexes: []int{0},
	}, {
		user:    "fred",
		indexes: []int{0},
	}}
	for i, test := range tests {
		c.Logf("test %d: as user %s", i, test.user)
		expectResp := &params.ListEnvironmentsResponse{
			Environments: make([]params.EnvironmentResponse, len(test.indexes)),
		}
		for i, index := range test.indexes {
			expectResp.Environments[i] = resps[index]
		}

		resp, err := s.client(test.user).ListEnvironments(nil)
		c.Assert(err, gc.IsNil)
		c.Assert(resp, jc.DeepEquals, expectResp)
	}
}

func (s *APISuite) TestGetSetStateServerPerm(c *gc.C) {
	srvId := s.addStateServer(c, params.EntityPath{"alice", "foo"})

	acl, err := s.client("alice").GetStateServerPerm(&params.GetStateServerPerm{
		EntityPath: srvId,
	})
	c.Assert(err, gc.IsNil)
	c.Assert(acl, gc.DeepEquals, params.ACL{})

	err = s.client("alice").SetStateServerPerm(&params.SetStateServerPerm{
		EntityPath: srvId,
		ACL: params.ACL{
			Read: []string{"a", "b"},
		},
	})
	c.Assert(err, gc.IsNil)
	acl, err = s.client("alice").GetStateServerPerm(&params.GetStateServerPerm{
		EntityPath: srvId,
	})
	c.Assert(err, gc.IsNil)
	c.Assert(acl, gc.DeepEquals, params.ACL{
		Read: []string{"a", "b"},
	})
}

func (s *APISuite) TestGetSetEnvironmentPerm(c *gc.C) {
	srvId := s.addStateServer(c, params.EntityPath{"alice", "foo"})

	acl, err := s.client("alice").GetEnvironmentPerm(&params.GetEnvironmentPerm{
		EntityPath: srvId,
	})
	c.Assert(err, gc.IsNil)
	c.Assert(acl, gc.DeepEquals, params.ACL{})

	err = s.client("alice").SetEnvironmentPerm(&params.SetEnvironmentPerm{
		EntityPath: srvId,
		ACL: params.ACL{
			Read: []string{"a", "b"},
		},
	})
	c.Assert(err, gc.IsNil)
	acl, err = s.client("alice").GetEnvironmentPerm(&params.GetEnvironmentPerm{
		EntityPath: srvId,
	})
	c.Assert(err, gc.IsNil)
	c.Assert(acl, gc.DeepEquals, params.ACL{
		Read: []string{"a", "b"},
	})
}

// addStateServer adds a new stateserver named name under the
// given user. It returns the state server id.
func (s *APISuite) addStateServer(c *gc.C, srvPath params.EntityPath) params.EntityPath {
	// Note that because the cookies acquired in this request don't
	// persist, the discharge macaroon we get won't affect subsequent
	// requests in the caller.

	info := s.APIInfo(c)
	err := s.client(srvPath.User).AddJES(&params.AddJES{
		EntityPath: srvPath,
		Info: params.ServerInfo{
			HostPorts:   info.Addrs,
			CACert:      info.CACert,
			User:        info.Tag.Id(),
			Password:    info.Password,
			EnvironUUID: info.EnvironTag.Id(),
		},
	})
	c.Assert(err, gc.IsNil)
	return srvPath
}

// addEnvironment adds a new environment in the given state server. It
// returns the environment id.
func (s *APISuite) addEnvironment(c *gc.C, srvPath, envPath params.EntityPath) (path params.EntityPath, user, uuid string) {
	// Note that because the cookies acquired in this request don't
	// persist, the discharge macaroon we get won't affect subsequent
	// requests in the caller.

	info := s.APIInfo(c)
	resp, err := s.client(envPath.User).NewEnvironment(&params.NewEnvironment{
		User: envPath.User,
		Info: params.NewEnvironmentInfo{
			Name:        envPath.Name,
			Password:    info.Password,
			StateServer: srvPath,
			Config:      dummyEnvConfig,
		},
	})
	c.Assert(err, gc.IsNil)
	return resp.Path, resp.User, resp.UUID
}

func bakeryDo(client *httpbakery.Client) func(*http.Request) (*http.Response, error) {
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

var anyBody = httptesting.BodyAsserter(func(*gc.C, json.RawMessage) {
})
