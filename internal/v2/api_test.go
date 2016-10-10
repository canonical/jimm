package v2_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/juju/juju/api"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/testing/httptesting"
	gc "gopkg.in/check.v1"
	"gopkg.in/errgo.v1"
	"gopkg.in/juju/names.v2"

	"github.com/CanonicalLtd/jem/internal/apitest"
	"github.com/CanonicalLtd/jem/internal/mongodoc"
	"github.com/CanonicalLtd/jem/params"
)

type APISuite struct {
	apitest.Suite
}

var _ = gc.Suite(&APISuite{})

const sshKey = "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQDOjaOjVRHchF2RFCKQdgBqrIA5nOoqSprLK47l2th5I675jw+QYMIihXQaITss3hjrh3+5ITyBO41PS5rHLNGtlYUHX78p9CHNZsJqHl/z1Ub1tuMe+/5SY2MkDYzgfPtQtVsLasAIiht/5g78AMMXH3HeCKb9V9cP6/lPPq6mCMvg8TDLrPp/P2vlyukAsJYUvVgoaPDUBpedHbkMj07pDJqe4D7c0yEJ8hQo/6nS+3bh9Q1NvmVNsB1pbtk3RKONIiTAXYcjclmOljxxJnl1O50F5sOIi38vyl7Q63f6a3bXMvJEf1lnPNJKAxspIfEu8gRasny3FEsbHfrxEwVj rog@rog-x220"

var dummyModelConfig = map[string]interface{}{
	"authorized-keys": sshKey,
	"controller":      true,
}

var unauthorizedTests = []struct {
	about  string
	asUser string
	method string
	path   string
	body   interface{}
}{{
	about:  "get model as non-owner",
	asUser: "other",
	method: "GET",
	path:   "/v2/model/bob/private",
}, {
	about:  "get controller as non-owner",
	asUser: "other",
	method: "GET",
	path:   "/v2/controller/bob/private",
}, {
	about:  "new model as non-owner",
	asUser: "other",
	method: "POST",
	path:   "/v2/model/bob",
	body: params.NewModelInfo{
		Name:       "newmodel",
		Controller: &params.EntityPath{"bob", "open"},
		Credential: "cred1",
	},
}, {
	about:  "new model with inaccessible controller",
	asUser: "alice",
	method: "POST",
	path:   "/v2/model/alice",
	body: params.NewModelInfo{
		Name:       "newmodel",
		Controller: &params.EntityPath{"bob", "private"},
		Credential: "cred1",
	},
}, {
	about:  "set controller perm as non-owner",
	asUser: "other",
	method: "PUT",
	path:   "/v2/controller/bob/open/perm",
	body:   params.ACL{},
}, {
	about:  "set model perm as non-owner",
	asUser: "other",
	method: "PUT",
	path:   "/v2/model/bob/open/perm",
	body:   params.ACL{},
}, {
	about:  "get controller perm as non-owner",
	asUser: "other",
	method: "GET",
	path:   "/v2/controller/bob/private/perm",
}, {
	about:  "get model perm as non-owner",
	asUser: "other",
	method: "GET",
	path:   "/v2/model/bob/private/perm",
}, {
	about:  "get controller perm with ACL that allows us",
	asUser: "other",
	method: "GET",
	path:   "/v2/controller/bob/open/perm",
}, {
	about:  "get model perm with ACL that allows us",
	asUser: "other",
	method: "GET",
	path:   "/v2/model/bob/open/perm",
}}

func (s *APISuite) TestUnauthorized(c *gc.C) {
	s.AssertAddController(c, params.EntityPath{"bob", "private"}, false)
	s.AssertAddController(c, params.EntityPath{"bob", "open"}, false)

	s.allowControllerPerm(c, params.EntityPath{"bob", "open"})
	s.allowModelPerm(c, params.EntityPath{"bob", "open"})

	for i, test := range unauthorizedTests {
		c.Logf("test %d: %s", i, test.about)
		httptesting.AssertJSONCall(c, httptesting.JSONCallParams{
			Method:   test.method,
			Handler:  s.JEMSrv,
			JSONBody: test.body,
			URL:      test.path,
			ExpectBody: &params.Error{
				Message: `unauthorized`,
				Code:    params.ErrUnauthorized,
			},
			ExpectStatus: http.StatusUnauthorized,
			Do:           apitest.Do(s.IDMSrv.Client(test.asUser)),
		})
	}
}

func (s *APISuite) TestAddController(c *gc.C) {
	info := s.APIInfo(c)
	var addControllerTests = []struct {
		about        string
		authUser     params.User
		username     params.User
		body         params.ControllerInfo
		expectStatus int
		expectBody   interface{}
	}{{
		about: "add private controller",
		body: params.ControllerInfo{
			HostPorts:      info.Addrs,
			CACert:         info.CACert,
			User:           info.Tag.Id(),
			Password:       info.Password,
			ControllerUUID: info.ModelTag.Id(),
			Cloud:          "dummy",
			Region:         "dummy-region",
		},
		expectStatus: http.StatusForbidden,
		expectBody: &params.Error{
			Message: "cannot add private controller",
			Code:    params.ErrForbidden,
		},
	}, {
		about:    "add controller as part of group",
		username: "beatles",
		authUser: "alice",
		body: params.ControllerInfo{
			HostPorts:      info.Addrs,
			CACert:         info.CACert,
			User:           info.Tag.Id(),
			Password:       info.Password,
			ControllerUUID: info.ModelTag.Id(),
			Public:         true,
			Cloud:          "dummy",
			Region:         "dummy-region",
		},
	}, {
		about:    "add public controller",
		username: "controller-admin",
		authUser: "controller-admin",
		body: params.ControllerInfo{
			HostPorts:      info.Addrs,
			CACert:         info.CACert,
			User:           info.Tag.Id(),
			Password:       info.Password,
			ControllerUUID: info.ModelTag.Id(),
			Public:         true,
			Cloud:          "dummy",
			Region:         "dummy-region",
		},
	}, {
		about:    "incorrect user",
		authUser: "alice",
		username: "bob",
		body: params.ControllerInfo{
			HostPorts:      info.Addrs,
			CACert:         info.CACert,
			User:           info.Tag.Id(),
			Password:       info.Password,
			ControllerUUID: info.ModelTag.Id(),
			Cloud:          "dummy",
			Region:         "dummy-region",
		},
		expectStatus: http.StatusUnauthorized,
		expectBody: params.Error{
			Code:    "unauthorized",
			Message: "unauthorized",
		},
	}, {
		about: "no hosts",
		body: params.ControllerInfo{
			CACert:         info.CACert,
			User:           info.Tag.Id(),
			Password:       info.Password,
			ControllerUUID: info.ModelTag.Id(),
			Public:         true,
			Cloud:          "dummy",
			Region:         "dummy-region",
		},
		expectStatus: http.StatusBadRequest,
		expectBody: params.Error{
			Code:    "bad request",
			Message: "no host-ports in request",
		},
	}, {
		about: "no user",
		body: params.ControllerInfo{
			HostPorts:      info.Addrs,
			CACert:         info.CACert,
			Password:       info.Password,
			ControllerUUID: info.ModelTag.Id(),
			Public:         true,
			Cloud:          "dummy",
			Region:         "dummy-region",
		},
		expectStatus: http.StatusBadRequest,
		expectBody: params.Error{
			Code:    "bad request",
			Message: "no user in request",
		},
	}, {
		about: "no model uuid",
		body: params.ControllerInfo{
			HostPorts: info.Addrs,
			CACert:    info.CACert,
			User:      info.Tag.Id(),
			Password:  info.Password,
			Public:    true,
			Cloud:     "dummy",
			Region:    "dummy-region",
		},
		expectStatus: http.StatusBadRequest,
		expectBody: params.Error{
			Code:    "bad request",
			Message: "bad model UUID in request",
		},
	}, {
		about:    "public but no controller-admin access",
		username: "bob",
		authUser: "bob",
		body: params.ControllerInfo{
			HostPorts: info.Addrs,
			CACert:    info.CACert,
			User:      info.Tag.Id(),
			Password:  info.Password,
			Public:    true,
			Cloud:     "dummy",
			Region:    "dummy-region",
		},
		expectStatus: http.StatusUnauthorized,
		expectBody: params.Error{
			Code:    params.ErrUnauthorized,
			Message: `admin access required to add public controllers`,
		},
	}, {
		about: "cannot connect to environment",
		body: params.ControllerInfo{
			HostPorts:      []string{"0.1.2.3:1234"},
			CACert:         info.CACert,
			User:           info.Tag.Id(),
			Password:       info.Password,
			ControllerUUID: info.ModelTag.Id(),
			Public:         true,
			Cloud:          "dummy",
			Region:         "dummy-region",
		},
		expectStatus: http.StatusBadRequest,
		expectBody: httptesting.BodyAsserter(func(c *gc.C, m json.RawMessage) {
			var body params.Error
			err := json.Unmarshal(m, &body)
			c.Assert(err, gc.IsNil)
			c.Assert(body.Code, gc.Equals, params.ErrBadRequest)
			c.Assert(body.Message, gc.Matches, `cannot connect to controller: cannot connect to API: unable to connect to API: .*`)
		}),
	}, {
		about: "no cloud",
		body: params.ControllerInfo{
			HostPorts:      []string{"0.1.2.3:1234"},
			CACert:         info.CACert,
			User:           info.Tag.Id(),
			Password:       info.Password,
			ControllerUUID: info.ModelTag.Id(),
			Public:         true,
		},
		expectStatus: http.StatusBadRequest,
		expectBody: params.Error{
			Code:    params.ErrBadRequest,
			Message: `cannot unmarshal parameters: cannot unmarshal into field: cannot unmarshal request body: invalid cloud ""`,
		},
	}, {
		about:    "no region",
		username: "controller-admin",
		authUser: "controller-admin",
		body: params.ControllerInfo{
			HostPorts:      info.Addrs,
			CACert:         info.CACert,
			User:           info.Tag.Id(),
			Password:       info.Password,
			ControllerUUID: info.ModelTag.Id(),
			Public:         true,
			Cloud:          "dummy",
		},
	}}
	s.IDMSrv.AddUser("alice", "beatles", "controller-admin")
	s.IDMSrv.AddUser("bob", "beatles")
	s.IDMSrv.AddUser("testuser", "controller-admin")
	for i, test := range addControllerTests {
		c.Logf("test %d: %s", i, test.about)
		controllerPath := params.EntityPath{
			User: test.username,
			Name: params.Name(fmt.Sprintf("controller%d", i)),
		}
		if controllerPath.User == "" {
			controllerPath.User = "testuser"
		}
		authUser := test.authUser
		if authUser == "" {
			authUser = controllerPath.User
		}
		client := s.IDMSrv.Client(string(authUser))
		httptesting.AssertJSONCall(c, httptesting.JSONCallParams{
			Method:       "PUT",
			Handler:      s.JEMSrv,
			JSONBody:     test.body,
			URL:          fmt.Sprintf("/v2/controller/%s", controllerPath),
			Do:           apitest.Do(client),
			ExpectStatus: test.expectStatus,
			ExpectBody:   test.expectBody,
		})
		if test.expectStatus != 0 {
			continue
		}
		// The controller was added successfully. Check that we
		// can connect to it.
		conn, err := s.JEM.OpenAPI(controllerPath)
		c.Assert(err, gc.IsNil)
		conn.Close()
		// Clear the connection pool for the next test.
		s.JEMSrv.Pool().ClearAPIConnCache()
	}
}

func (s *APISuite) TestAddControllerDuplicate(c *gc.C) {
	ctlPath := s.AssertAddController(c, params.EntityPath{"bob", "dupmodel"}, false)
	err := s.AddController(c, ctlPath, false)
	c.Assert(err, gc.ErrorMatches, "already exists")
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrAlreadyExists)
}

func (s *APISuite) TestAddControllerUnauthenticated(c *gc.C) {
	httptesting.AssertJSONCall(c, httptesting.JSONCallParams{
		Method:  "PUT",
		Handler: s.JEMSrv,
		URL:     "/v2/controller/user/model",
		ExpectBody: httptesting.BodyAsserter(func(c *gc.C, m json.RawMessage) {
			// Allow any body - TestGetModelNotFound will check that it's a valid macaroon.
		}),
		ExpectStatus: http.StatusProxyAuthRequired,
	})
}

func (s *APISuite) TestAddControllerUnauthenticatedWithBakeryProtocol(c *gc.C) {
	httptesting.AssertJSONCall(c, httptesting.JSONCallParams{
		Method:  "PUT",
		Handler: s.JEMSrv,
		Header:  map[string][]string{"Bakery-Protocol-Version": {"1"}},
		URL:     "/v2/controller/user/model",
		ExpectBody: httptesting.BodyAsserter(func(c *gc.C, m json.RawMessage) {
			// Allow any body - TestGetModelNotFound will check that it's a valid macaroon.
		}),
		ExpectStatus: http.StatusUnauthorized,
	})
}

func (s *APISuite) TestGetModelNotFound(c *gc.C) {
	httptesting.AssertJSONCall(c, httptesting.JSONCallParams{
		Method:  "GET",
		Handler: s.JEMSrv,
		URL:     "/v2/model/user/foo",
		ExpectBody: &params.Error{
			Message: `model "user/foo" not found`,
			Code:    params.ErrNotFound,
		},
		ExpectStatus: http.StatusNotFound,
		Do:           apitest.Do(s.IDMSrv.Client("user")),
	})

	// If we're some different user, we get Unauthorized.
	httptesting.AssertJSONCall(c, httptesting.JSONCallParams{
		Method:  "GET",
		Handler: s.JEMSrv,
		URL:     "/v2/model/user/foo",
		ExpectBody: &params.Error{
			Message: `unauthorized`,
			Code:    params.ErrUnauthorized,
		},
		ExpectStatus: http.StatusUnauthorized,
		Do:           apitest.Do(s.IDMSrv.Client("other")),
	})
}

func (s *APISuite) TestDeleteModelNotFound(c *gc.C) {
	httptesting.AssertJSONCall(c, httptesting.JSONCallParams{
		Method:  "DELETE",
		Handler: s.JEMSrv,
		URL:     "/v2/model/user/foo",
		ExpectBody: &params.Error{
			Message: `model "user/foo" not found`,
			Code:    params.ErrNotFound,
		},
		ExpectStatus: http.StatusNotFound,
		Do:           apitest.Do(s.IDMSrv.Client("user")),
	})
}

func (s *APISuite) TestDeleteModel(c *gc.C) {
	ctlId := s.AssertAddController(c, params.EntityPath{"bob", "who"}, false)
	cred := s.AssertUpdateCredential(c, "bob", "dummy", "cred1", "empty")
	modelPath := params.EntityPath{"bob", "foobarred"}
	modelId, uuid := s.CreateModel(c, modelPath, ctlId, cred)
	resp := httptesting.DoRequest(c, httptesting.DoRequestParams{
		Handler: s.JEMSrv,
		URL:     "/v2/model/" + modelId.String(),
		Do:      apitest.Do(s.IDMSrv.Client("bob")),
	})
	c.Assert(resp.Code, gc.Equals, http.StatusOK, gc.Commentf("body: %s", resp.Body.Bytes()))
	c.Assert(uuid, gc.NotNil)

	// Delete model.
	httptesting.AssertJSONCall(c, httptesting.JSONCallParams{
		Method:       "DELETE",
		Handler:      s.JEMSrv,
		URL:          "/v2/model/" + modelId.String(),
		ExpectStatus: http.StatusOK,
		Do:           apitest.Do(s.IDMSrv.Client("bob")),
	})
	// Check that it doesn't exist anymore.
	httptesting.AssertJSONCall(c, httptesting.JSONCallParams{
		Handler:      s.JEMSrv,
		URL:          "/v2/model/" + modelId.String(),
		ExpectStatus: http.StatusNotFound,
		ExpectBody: &params.Error{
			Message: `model "bob/foobarred" not found`,
			Code:    params.ErrNotFound,
		},
		Do: apitest.Do(s.IDMSrv.Client("bob")),
	})
}

func (s *APISuite) TestGetController(c *gc.C) {
	ctlId := s.AssertAddController(c, params.EntityPath{"bob", "foo"}, false)
	t := time.Now()
	err := s.JEM.DB.SetControllerUnavailableAt(ctlId, t)
	c.Assert(err, gc.IsNil)

	resp := httptesting.DoRequest(c, httptesting.DoRequestParams{
		Handler: s.JEMSrv,
		URL:     "/v2/controller/" + ctlId.String(),
		Do:      apitest.Do(s.IDMSrv.Client("bob")),
	})
	c.Assert(resp.Code, gc.Equals, http.StatusOK, gc.Commentf("body: %s", resp.Body.Bytes()))
	var controllerInfo params.ControllerResponse
	err = json.Unmarshal(resp.Body.Bytes(), &controllerInfo)
	c.Assert(err, gc.IsNil, gc.Commentf("body: %s", resp.Body.String()))
	c.Assert(controllerInfo.ProviderType, gc.Equals, "dummy")
	c.Assert(controllerInfo.Schema, gc.Not(gc.HasLen), 0)
	// Check that all path attributes have been removed.
	for name := range controllerInfo.Schema {
		c.Assert(strings.HasSuffix(name, "-path"), gc.Equals, false)
	}
	c.Assert((*controllerInfo.UnavailableSince).UTC(), jc.DeepEquals, mongodoc.Time(t).UTC())
	c.Logf("%#v", controllerInfo.Schema)
}

func (s *APISuite) TestGetControllerWithLocation(c *gc.C) {
	s.IDMSrv.AddUser("bob", "controller-admin")
	ctlId := params.EntityPath{"bob", "foo"}
	info := s.APIInfo(c)

	err := s.NewClient(ctlId.User).AddController(&params.AddController{
		EntityPath: ctlId,
		Info: params.ControllerInfo{
			HostPorts:      info.Addrs,
			CACert:         info.CACert,
			User:           info.Tag.Id(),
			Password:       info.Password,
			ControllerUUID: info.ModelTag.Id(),
			Public:         true,
			Cloud:          "dummy",
			Region:         "dummy-region",
		},
	})
	c.Assert(err, gc.IsNil)

	resp := httptesting.DoRequest(c, httptesting.DoRequestParams{
		Handler: s.JEMSrv,
		URL:     "/v2/controller/" + ctlId.String(),
		Do:      apitest.Do(s.IDMSrv.Client("bob")),
	})
	c.Assert(resp.Code, gc.Equals, http.StatusOK, gc.Commentf("body: %s", resp.Body.Bytes()))
	var controllerInfo params.ControllerResponse
	err = json.Unmarshal(resp.Body.Bytes(), &controllerInfo)
	c.Assert(err, gc.IsNil, gc.Commentf("body: %s", resp.Body.String()))
	c.Assert(controllerInfo.Public, gc.Equals, true)
}

func (s *APISuite) TestGetControllerLocation(c *gc.C) {
	ctlId := params.EntityPath{"bob", "foo"}

	s.AssertAddController(c, ctlId, false)

	// Check the location attributes.
	httptesting.AssertJSONCall(c, httptesting.JSONCallParams{
		Method:       "GET",
		Handler:      s.JEMSrv,
		URL:          "/v2/controller/" + ctlId.String() + "/meta/location",
		ExpectStatus: http.StatusOK,
		ExpectBody: params.ControllerLocation{
			Location: map[string]string{"cloud": "dummy", "region": "dummy-region"},
		},
		Do: apitest.Do(s.IDMSrv.Client("bob")),
	})

	// Check alice can't access them.
	httptesting.AssertJSONCall(c, httptesting.JSONCallParams{
		Method:       "GET",
		Handler:      s.JEMSrv,
		URL:          "/v2/controller/" + ctlId.String() + "/meta/location",
		ExpectStatus: http.StatusUnauthorized,
		ExpectBody: &params.Error{
			Message: `unauthorized`,
			Code:    params.ErrUnauthorized,
		},
		Do: apitest.Do(s.IDMSrv.Client("alice")),
	})

	// Check alice can't probe controllers througth GET.
	httptesting.AssertJSONCall(c, httptesting.JSONCallParams{
		Method:       "GET",
		Handler:      s.JEMSrv,
		URL:          "/v2/controller/bob/notexist/meta/location",
		ExpectStatus: http.StatusUnauthorized,
		ExpectBody: &params.Error{
			Message: `unauthorized`,
			Code:    params.ErrUnauthorized,
		},
		Do: apitest.Do(s.IDMSrv.Client("alice")),
	})
}

func (s *APISuite) TestGetControllerNotFound(c *gc.C) {
	httptesting.AssertJSONCall(c, httptesting.JSONCallParams{
		Method:  "GET",
		Handler: s.JEMSrv,
		URL:     "/v2/controller/bob/foo",
		ExpectBody: &params.Error{
			Message: `controller "bob/foo" not found`,
			Code:    params.ErrNotFound,
		},
		ExpectStatus: http.StatusNotFound,
		Do:           apitest.Do(s.IDMSrv.Client("bob")),
	})

	// Any other user just sees Unauthorized.
	httptesting.AssertJSONCall(c, httptesting.JSONCallParams{
		Method:  "GET",
		Handler: s.JEMSrv,
		URL:     "/v2/controller/bob/foo",
		ExpectBody: &params.Error{
			Message: `unauthorized`,
			Code:    params.ErrUnauthorized,
		},
		ExpectStatus: http.StatusUnauthorized,
		Do:           apitest.Do(s.IDMSrv.Client("alice")),
	})
}

func (s *APISuite) TestDeleteControllerNotFound(c *gc.C) {
	httptesting.AssertJSONCall(c, httptesting.JSONCallParams{
		Method:  "DELETE",
		Handler: s.JEMSrv,
		URL:     "/v2/controller/bob/foobarred",
		ExpectBody: &params.Error{
			Message: `controller "bob/foobarred" not found`,
			Code:    params.ErrNotFound,
		},
		ExpectStatus: http.StatusNotFound,
		Do:           apitest.Do(s.IDMSrv.Client("bob")),
	})
}

func (s *APISuite) TestDeleteController(c *gc.C) {
	// Add controller to JEM.
	ctlId := s.AssertAddController(c, params.EntityPath{"bob", "foobarred"}, false)
	// Assert that it was added.
	resp := httptesting.DoRequest(c, httptesting.DoRequestParams{
		Handler: s.JEMSrv,
		URL:     "/v2/controller/" + ctlId.String(),
		Do:      apitest.Do(s.IDMSrv.Client("bob")),
	})
	c.Assert(resp.Code, gc.Equals, http.StatusOK, gc.Commentf("body: %s", resp.Body.Bytes()))
	cred := s.AssertUpdateCredential(c, "bob", "dummy", "cred1", "empty")
	// Add another model to it.
	modelId, _ := s.CreateModel(c, params.EntityPath{"bob", "bar"}, ctlId, cred)

	// Check that we can't delete it because it's marked as "available"
	httptesting.AssertJSONCall(c, httptesting.JSONCallParams{
		Method:       "DELETE",
		Handler:      s.JEMSrv,
		URL:          "/v2/controller/bob/foobarred",
		ExpectStatus: http.StatusForbidden,
		ExpectBody: params.Error{
			Message: `cannot delete controller while it is still alive`,
			Code:    params.ErrStillAlive,
		},
		Do: apitest.Do(s.IDMSrv.Client("bob")),
	})

	// Check that we can delete it with force flag.
	httptesting.AssertJSONCall(c, httptesting.JSONCallParams{
		Method:       "DELETE",
		Handler:      s.JEMSrv,
		URL:          "/v2/controller/bob/foobarred?force=true",
		ExpectStatus: http.StatusOK,
		Do:           apitest.Do(s.IDMSrv.Client("bob")),
	})
	// Check that it doesn't exist anymore.
	httptesting.AssertJSONCall(c, httptesting.JSONCallParams{
		Handler:      s.JEMSrv,
		URL:          "/v2/controller/" + ctlId.String(),
		Do:           apitest.Do(s.IDMSrv.Client("bob")),
		ExpectStatus: http.StatusNotFound,
		ExpectBody: params.Error{
			Message: `controller "bob/foobarred" not found`,
			Code:    params.ErrNotFound,
		},
	})
	// Check that its models doesn't exist.
	httptesting.AssertJSONCall(c, httptesting.JSONCallParams{
		Handler:      s.JEMSrv,
		URL:          "/v2/model/" + ctlId.String(),
		Do:           apitest.Do(s.IDMSrv.Client("bob")),
		ExpectStatus: http.StatusNotFound,
		ExpectBody: params.Error{
			Message: `model "bob/foobarred" not found`,
			Code:    params.ErrNotFound,
		},
	})
	httptesting.AssertJSONCall(c, httptesting.JSONCallParams{
		Handler:      s.JEMSrv,
		URL:          "/v2/model/" + modelId.String(),
		Do:           apitest.Do(s.IDMSrv.Client("bob")),
		ExpectStatus: http.StatusNotFound,
		ExpectBody: params.Error{
			Message: `model "bob/bar" not found`,
			Code:    params.ErrNotFound,
		},
	})
}

var getControllerLocationsTests = []struct {
	about       string
	arg         params.GetControllerLocations
	user        params.User
	expect      params.ControllerLocationsResponse
	expectError string
	expectCause error
}{{
	about: "no filters",
	user:  "bob",
	arg: params.GetControllerLocations{
		Attr: "cloud",
	},
	expect: params.ControllerLocationsResponse{
		Values: []string{"aws", "gce"},
	},
}, {
	about: "filter to single cloud",
	user:  "bob",
	arg: params.GetControllerLocations{
		Attr: "region",
		Location: map[string]string{
			"cloud": "aws",
		},
	},
	expect: params.ControllerLocationsResponse{
		Values: []string{"eu-west-1", "us-east-1"},
	},
}, {
	about: "no matching controllers",
	user:  "bob",
	arg: params.GetControllerLocations{
		Attr: "region",
		Location: map[string]string{
			"cloud": "joyent",
		},
	},
	expect: params.ControllerLocationsResponse{},
}, {
	about: "invalid filter attribute",
	user:  "bob",
	arg: params.GetControllerLocations{
		Attr: "region",
		Location: map[string]string{
			"cloud.blah": "aws",
		},
	},
	expectError: `invalid location attribute "cloud\.blah"`,
	expectCause: params.ErrBadRequest,
}, {
	about: "user without access to everything",
	user:  "alice",
	arg: params.GetControllerLocations{
		Attr: "cloud",
	},
	expect: params.ControllerLocationsResponse{
		Values: []string{"azure", "gce"},
	},
}}

func (s *APISuite) TestGetControllerLocations(c *gc.C) {
	s.AssertAddControllerDoc(c, &mongodoc.Controller{
		Path: params.EntityPath{"bob", "aws-us-east"},
		Cloud: mongodoc.Cloud{
			Name: "aws",
			Regions: []mongodoc.Region{{
				Name: "us-east-1",
			}},
		},
		Public: true,
	})
	s.AssertAddControllerDoc(c, &mongodoc.Controller{
		Path: params.EntityPath{"bob", "aws-us-east2"},
		Cloud: mongodoc.Cloud{
			Name: "aws",
			Regions: []mongodoc.Region{{
				Name: "us-east-1",
			}},
		},
		Public: true,
	})
	s.AssertAddControllerDoc(c, &mongodoc.Controller{
		Path: params.EntityPath{"bob", "aws-eu-west"},
		Cloud: mongodoc.Cloud{
			Name: "aws",
			Regions: []mongodoc.Region{{
				Name: "eu-west-1",
			}},
		},
		Public: true,
	})
	s.AssertAddControllerDoc(c, &mongodoc.Controller{
		Path: params.EntityPath{"bob", "gce-somewhere"},
		Cloud: mongodoc.Cloud{
			Name: "gce",
			Regions: []mongodoc.Region{{
				Name: "somewhere",
			}},
		},
		Public: true,
	})
	ctlId := params.EntityPath{"bob", "gce-down"}
	s.AssertAddControllerDoc(c, &mongodoc.Controller{
		Path: ctlId,
		Cloud: mongodoc.Cloud{
			Name: "gce",
			Regions: []mongodoc.Region{{
				Name: "down",
			}},
		},
		Public: true,
	})
	err := s.JEM.DB.SetControllerUnavailableAt(ctlId, time.Now())
	c.Assert(err, gc.IsNil)
	s.AssertAddControllerDoc(c, &mongodoc.Controller{
		Path: params.EntityPath{"bob", "gce-elsewhere"},
		Cloud: mongodoc.Cloud{
			Name: "gce",
			Regions: []mongodoc.Region{{
				Name: "elsewhere",
			}},
		},
		Public: true,
	})
	s.IDMSrv.AddUser("alice", "somegroup")
	s.AssertAddControllerDoc(c, &mongodoc.Controller{
		Path: params.EntityPath{"somegroup", "controller"},
		Cloud: mongodoc.Cloud{
			Name: "gce",
			Regions: []mongodoc.Region{{
				Name: "america",
			}},
		},
		Public: true,
	})
	s.AssertAddControllerDoc(c, &mongodoc.Controller{
		Path: params.EntityPath{"alice", "controller"},
		Cloud: mongodoc.Cloud{
			Name: "azure",
			Regions: []mongodoc.Region{{
				Name: "america",
			}},
		},
		Public: true,
	})
	s.AssertAddControllerDoc(c, &mongodoc.Controller{
		Path: params.EntityPath{"alice", "forgotten"},
		Cloud: mongodoc.Cloud{
			Name: "azure",
			Regions: []mongodoc.Region{{
				Name: "america",
			}},
		},
		Public: false,
	})

	for i, test := range getControllerLocationsTests {
		c.Logf("test %d: %v", i, test.about)
		resp, err := s.NewClient(test.user).GetControllerLocations(&test.arg)
		if test.expectError != "" {
			c.Check(resp, gc.IsNil)
			c.Assert(err, gc.ErrorMatches, test.expectError)
			c.Assert(errgo.Cause(err), gc.Equals, test.expectCause)
			continue
		}
		c.Assert(err, gc.IsNil)
		c.Assert(resp, jc.DeepEquals, &test.expect)
	}
}

var getAllControllerLocationsTests = []struct {
	about       string
	arg         params.GetAllControllerLocations
	user        params.User
	expect      params.AllControllerLocationsResponse
	expectError string
	expectCause error
}{{
	about: "no filters",
	user:  "bob",
	expect: params.AllControllerLocationsResponse{
		Locations: []map[string]string{{
			"cloud":  "aws",
			"region": "eu-west-1",
		}, {
			"cloud":  "aws",
			"region": "us-east-1",
		}, {
			"cloud":  "gce",
			"region": "elsewhere",
		}, {
			"cloud":  "gce",
			"region": "somewhere",
		}},
	},
}, {
	about: "filter to single cloud",
	user:  "bob",
	arg: params.GetAllControllerLocations{
		Location: map[string]string{
			"cloud": "aws",
		},
	},
	expect: params.AllControllerLocationsResponse{
		Locations: []map[string]string{{
			"cloud":  "aws",
			"region": "eu-west-1",
		}, {
			"cloud":  "aws",
			"region": "us-east-1",
		}},
	},
}, {
	about: "no matching controllers",
	user:  "bob",
	arg: params.GetAllControllerLocations{
		Location: map[string]string{
			"cloud": "joyent",
		},
	},
	expect: params.AllControllerLocationsResponse{},
}, {
	about: "invalid filter attribute",
	user:  "bob",
	arg: params.GetAllControllerLocations{
		Location: map[string]string{
			"cloud.blah": "aws",
		},
	},
	expectError: `invalid location attribute "cloud\.blah"`,
	expectCause: params.ErrBadRequest,
}, {
	about: "user without access to everything",
	user:  "alice",
	expect: params.AllControllerLocationsResponse{
		Locations: []map[string]string{{
			"cloud":  "azure",
			"region": "america",
		}},
	},
}}

func (s *APISuite) TestAllControllerLocations(c *gc.C) {
	s.AssertAddControllerDoc(c, &mongodoc.Controller{
		Path: params.EntityPath{"bob", "aws-us-east"},
		Cloud: mongodoc.Cloud{
			Name: "aws",
			Regions: []mongodoc.Region{{
				Name: "us-east-1",
			}},
		},
		Public: true,
	})
	s.AssertAddControllerDoc(c, &mongodoc.Controller{
		Path: params.EntityPath{"bob", "aws-us-east2"},
		Cloud: mongodoc.Cloud{
			Name: "aws",
			Regions: []mongodoc.Region{{
				Name: "us-east-1",
			}},
		},
		Public: true,
	})
	s.AssertAddControllerDoc(c, &mongodoc.Controller{
		Path: params.EntityPath{"bob", "aws-eu-west"},
		Cloud: mongodoc.Cloud{
			Name: "aws",
			Regions: []mongodoc.Region{{
				Name: "eu-west-1",
			}},
		},
		Public: true,
	})
	s.AssertAddControllerDoc(c, &mongodoc.Controller{
		Path: params.EntityPath{"bob", "gce-somewhere"},
		Cloud: mongodoc.Cloud{
			Name: "gce",
			Regions: []mongodoc.Region{{
				Name: "somewhere",
			}},
		},
		Public: true,
	})
	ctlId := params.EntityPath{"bob", "gce-down"}
	s.AssertAddControllerDoc(c, &mongodoc.Controller{
		Path: ctlId,
		Cloud: mongodoc.Cloud{
			Name: "gce",
			Regions: []mongodoc.Region{{
				Name: "down",
			}},
		},
		Public: true,
	})
	err := s.JEM.DB.SetControllerUnavailableAt(ctlId, time.Now())
	c.Assert(err, gc.IsNil)
	s.AssertAddControllerDoc(c, &mongodoc.Controller{
		Path: params.EntityPath{"bob", "gce-elsewhere"},
		Cloud: mongodoc.Cloud{
			Name: "gce",
			Regions: []mongodoc.Region{{
				Name: "elsewhere",
			}},
		},
		Public: true,
	})
	s.IDMSrv.AddUser("alice", "somegroup")
	s.AssertAddControllerDoc(c, &mongodoc.Controller{
		Path: params.EntityPath{"alice", "controller"},
		Cloud: mongodoc.Cloud{
			Name: "azure",
			Regions: []mongodoc.Region{{
				Name: "america",
			}},
		},
		Public: true,
	})
	s.AssertAddControllerDoc(c, &mongodoc.Controller{
		Path: params.EntityPath{"alice", "forgotten"},
		Cloud: mongodoc.Cloud{
			Name: "azure",
			Regions: []mongodoc.Region{{
				Name: "america",
			}},
		},
		Public: false,
	})

	for i, test := range getAllControllerLocationsTests {
		c.Logf("test %d: %v", i, test.about)
		resp, err := s.NewClient(test.user).GetAllControllerLocations(&test.arg)
		if test.expectError != "" {
			c.Check(resp, gc.IsNil)
			c.Assert(err, gc.ErrorMatches, test.expectError)
			c.Assert(errgo.Cause(err), gc.Equals, test.expectCause)
			continue
		}
		c.Assert(err, gc.IsNil)
		c.Assert(resp, jc.DeepEquals, &test.expect)
	}

}

func (s *APISuite) TestGetSchemaOneProviderType(c *gc.C) {
	ctlId := s.AssertAddController(c, params.EntityPath{"bob", "aws-us-east"}, true)
	s.AssertAddController(c, params.EntityPath{"bob", "aws-eu-west"}, true)
	ctl, err := s.NewClient("bob").GetController(&params.GetController{
		EntityPath: ctlId,
	})
	c.Assert(err, gc.IsNil)
	resp, err := s.NewClient("bob").GetSchema(&params.GetSchema{
		Location: map[string]string{
			"cloud": "dummy",
		},
	})
	c.Assert(err, gc.IsNil)

	c.Assert(resp.ProviderType, gc.Equals, ctl.ProviderType)
	c.Assert(resp.Schema, jc.DeepEquals, ctl.Schema)
}

func (s *APISuite) TestGetSchemaNotFound(c *gc.C) {
	s.AssertAddController(c, params.EntityPath{"bob", "aws-us-east"}, true)
	resp, err := s.NewClient("bob").GetSchema(&params.GetSchema{
		Location: map[string]string{
			"cloud": "ec2",
		},
	})
	c.Check(resp, gc.IsNil)
	c.Check(errgo.Cause(err), gc.Equals, params.ErrNotFound)
	c.Assert(err, gc.ErrorMatches, `no matching controllers`)
}

func (s *APISuite) TestGetSchemaAmbiguous(c *gc.C) {
	s.AssertAddController(c, params.EntityPath{"bob", "aws-us-east"}, true)
	// Add a controller directly to the database because we can only
	// have dummy provider controllers otherwise and we need one
	// of a different type.
	s.AssertAddControllerDoc(c, &mongodoc.Controller{
		Path: params.EntityPath{"bob", "azure"},
		Cloud: mongodoc.Cloud{
			Name:         "dummy",
			ProviderType: "another",
		},
		Public: true,
	})

	resp, err := s.NewClient("bob").GetSchema(&params.GetSchema{
		Location: map[string]string{
			"cloud": "dummy",
		},
	})
	c.Check(resp, gc.IsNil)
	c.Check(errgo.Cause(err), gc.Equals, params.ErrAmbiguousLocation)
	c.Assert(err, gc.ErrorMatches, `ambiguous location matches controller of more than one type`)
}

func (s *APISuite) TestGetSchemaBadLocation(c *gc.C) {
	resp, err := s.NewClient("bob").GetSchema(&params.GetSchema{
		Location: map[string]string{
			"$badlocation": "aws",
		},
	})
	c.Check(resp, gc.IsNil)
	c.Check(errgo.Cause(err), gc.Equals, params.ErrBadRequest)
	c.Assert(err, gc.ErrorMatches, `invalid location attribute "\$badlocation"`)
}

func (s *APISuite) TestNewModel(c *gc.C) {
	ctlId := s.AssertAddController(c, params.EntityPath{"bob", "foo"}, false)
	cred := s.AssertUpdateCredential(c, "bob", "dummy", "cred1", "empty")

	var modelRespBody json.RawMessage
	httptesting.AssertJSONCall(c, httptesting.JSONCallParams{
		Method:  "POST",
		URL:     "/v2/model/bob",
		Handler: s.JEMSrv,
		JSONBody: params.NewModelInfo{
			Name:       params.Name("bar"),
			Controller: &ctlId,
			Credential: cred,
			Location: map[string]string{
				"cloud": "dummy",
			},
			Config: dummyModelConfig,
		},
		ExpectBody: httptesting.BodyAsserter(func(_ *gc.C, body json.RawMessage) {
			modelRespBody = body
		}),
		Do: apitest.Do(s.IDMSrv.Client("bob")),
	})
	var modelResp params.ModelResponse
	err := json.Unmarshal(modelRespBody, &modelResp)
	c.Assert(err, gc.IsNil)

	st := s.openAPIFromModelResponse(c, &modelResp, "bob")
	defer st.Close()

	muuid, valid := st.Client().ModelUUID()
	c.Assert(muuid, gc.Not(gc.Equals), "")
	c.Assert(valid, gc.Equals, true)
	c.Assert(muuid, gc.Not(gc.Equals), s.APIInfo(c).ModelTag.Id())

	// Ensure that we can connect to the new model
	// from the information returned by GetModel.
	modelResp2, err := s.NewClient("bob").GetModel(&params.GetModel{
		EntityPath: params.EntityPath{
			User: "bob",
			Name: "bar",
		},
	})
	c.Assert(err, gc.IsNil)
	st = s.openAPIFromModelResponse(c, modelResp2, "bob")
	defer st.Close()
	muuid2, valid2 := st.Client().ModelUUID()
	c.Assert(valid2, gc.Equals, true)
	c.Assert(muuid2, gc.Equals, muuid)
}

var newModelWithoutExplicitControllerTests = []struct {
	about            string
	user             params.User
	info             params.NewModelInfo
	expectError      string
	expectErrorCause error
}{{
	about: "success",
	user:  "alice",
	info: params.NewModelInfo{
		Name:       "test-model",
		Credential: "cred1",
		Location: map[string]string{
			"cloud": "dummy",
		},
		Config: map[string]interface{}{
			"secret": "a secret",
		},
	},
}, {
	about: "no matching cloud",
	user:  "alice",
	info: params.NewModelInfo{
		Name:       "test-model",
		Credential: "cred1",
		Location: map[string]string{
			"cloud": "aws",
		},
		Config: map[string]interface{}{
			"secret": "a secret",
		},
	},
	expectError:      `cannot select controller: no matching controllers found`,
	expectErrorCause: params.ErrNotFound,
}, {
	about: "no matching region",
	user:  "alice",
	info: params.NewModelInfo{
		Name:       "test-model",
		Credential: "cred1",
		Location: map[string]string{
			"region": "us-east-1",
		},
		Config: map[string]interface{}{
			"secret": "a secret",
		},
	},
	expectError:      `cannot select controller: no matching controllers found`,
	expectErrorCause: params.ErrNotFound,
}, {
	about: "unrecognised location parameter",
	user:  "alice",
	info: params.NewModelInfo{
		Name:       "test-model",
		Credential: "cred1",
		Location: map[string]string{
			"dimension": "5th",
		},
		Config: map[string]interface{}{
			"secret": "a secret",
		},
	},
	expectError:      `cannot select controller: no matching controllers found`,
	expectErrorCause: params.ErrNotFound,
}, {
	about: "invalid location parameter",
	user:  "alice",
	info: params.NewModelInfo{
		Name:       "test-model",
		Credential: "cred1",
		Location: map[string]string{
			"cloud.blah": "dummy",
		},
		Config: map[string]interface{}{
			"secret": "a secret",
		},
	},
	expectError:      `cannot select controller: no matching controllers found`,
	expectErrorCause: params.ErrNotFound,
}, {
	about: "invalid cloud name",
	user:  "alice",
	info: params.NewModelInfo{
		Name:       "test-model",
		Credential: "cred1",
		Location: map[string]string{
			"cloud": "bad/name",
		},
		Config: map[string]interface{}{
			"secret": "a secret",
		},
	},
	expectError:      `cannot select controller: invalid cloud "bad/name"`,
	expectErrorCause: params.ErrBadRequest,
}}

func (s *APISuite) TestNewModelWithoutExplicitController(c *gc.C) {
	ctlId := s.AssertAddController(c, params.EntityPath{"bob", "foo"}, true)
	s.AssertUpdateCredential(c, "alice", "dummy", "cred1", "empty")
	s.allowControllerPerm(c, ctlId)

	for i, test := range newModelWithoutExplicitControllerTests {
		c.Logf("test %d. %s", i, test.about)
		test.info.Name = params.Name(fmt.Sprintf("test-model-%d", i))
		resp, err := s.NewClient(test.user).NewModel(&params.NewModel{
			User: test.user,
			Info: test.info,
		})
		if test.expectError != "" {
			c.Assert(err, gc.ErrorMatches, test.expectError)
			c.Assert(errgo.Cause(err), gc.Equals, test.expectErrorCause)
			continue
		}
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(resp.Path, jc.DeepEquals, params.EntityPath{test.user, test.info.Name})
		c.Assert(resp.ControllerPath, jc.DeepEquals, ctlId)
	}
}

func (s *APISuite) assertModelConfigAttr(c *gc.C, modelPath params.EntityPath, attr string, val interface{}) {
	m, err := s.JEM.DB.Model(modelPath)
	c.Assert(err, gc.IsNil)
	st, err := s.State.ForModel(names.NewModelTag(m.UUID))
	c.Assert(err, gc.IsNil)
	defer st.Close()
	stm, err := st.Model()
	c.Assert(err, gc.IsNil)
	cfg, err := stm.Config()
	c.Assert(err, gc.IsNil)
	c.Assert(cfg.AllAttrs()[attr], jc.DeepEquals, val)
}

func (s *APISuite) TestGetModel(c *gc.C) {
	info := s.APIInfo(c)
	ctlId := s.AssertAddController(c, params.EntityPath{"bob", "foo"}, false)
	err := s.JEM.DB.SetModelLife(ctlId, info.ModelTag.Id(), "dying")
	c.Assert(err, gc.IsNil)
	t := time.Now()
	err = s.JEM.DB.SetControllerUnavailableAt(ctlId, t)
	c.Assert(err, gc.IsNil)

	var modelRespBody json.RawMessage
	httptesting.AssertJSONCall(c, httptesting.JSONCallParams{
		Method:  "GET",
		Handler: s.JEMSrv,
		URL:     "/v2/model/bob/foo",
		ExpectBody: httptesting.BodyAsserter(func(_ *gc.C, body json.RawMessage) {
			modelRespBody = body
		}),
		Do: apitest.Do(s.IDMSrv.Client("bob")),
	})
	var modelResp params.ModelResponse
	err = json.Unmarshal(modelRespBody, &modelResp)
	c.Assert(err, gc.IsNil)
	c.Assert(modelResp, jc.DeepEquals, params.ModelResponse{
		Path:             ctlId,
		UUID:             info.ModelTag.Id(),
		ControllerPath:   ctlId,
		ControllerUUID:   s.ControllerConfig.ControllerUUID(),
		CACert:           info.CACert,
		HostPorts:        info.Addrs,
		Life:             "dying",
		UnavailableSince: newTime(mongodoc.Time(t).UTC()),
	})
}

func newTime(t time.Time) *time.Time {
	return &t
}

func (s *APISuite) openAPIFromModelResponse(c *gc.C, resp *params.ModelResponse, username string) api.Connection {
	st, err := api.Open(apiInfoFromModelResponse(resp), api.DialOpts{
		BakeryClient: s.IDMSrv.Client(username),
	})
	c.Assert(err, gc.IsNil)
	return st
}

func apiInfoFromModelResponse(resp *params.ModelResponse) *api.Info {
	return &api.Info{
		Addrs:    resp.HostPorts,
		CACert:   resp.CACert,
		ModelTag: names.NewModelTag(resp.UUID),
	}
}

func (s *APISuite) TestNewModelUnderGroup(c *gc.C) {
	s.IDMSrv.AddUser("bob", "beatles")
	ctlId := s.AssertAddController(c, params.EntityPath{"bob", "foo"}, false)
	cred := s.AssertUpdateCredential(c, "beatles", "dummy", "cred1", "empty")

	var modelRespBody json.RawMessage
	httptesting.AssertJSONCall(c, httptesting.JSONCallParams{
		Method:  "POST",
		URL:     "/v2/model/beatles",
		Handler: s.JEMSrv,
		JSONBody: params.NewModelInfo{
			Name:       params.Name("bar"),
			Controller: &ctlId,
			Credential: cred,
			Location: map[string]string{
				"cloud": "dummy",
			},
			Config: dummyModelConfig,
		},
		ExpectBody: httptesting.BodyAsserter(func(_ *gc.C, body json.RawMessage) {
			modelRespBody = body
		}),
		Do: apitest.Do(s.IDMSrv.Client("bob")),
	})
	var modelResp params.ModelResponse
	err := json.Unmarshal(modelRespBody, &modelResp)
	c.Assert(err, gc.IsNil)

	// Ensure that we can connect to the new model
	// Note: juju controllers cannot check groups yet, so we connect
	// directly with a username that is the group name.
	st := s.openAPIFromModelResponse(c, &modelResp, "beatles")
	st.Close()
}

var newModelWithInvalidControllerPathTests = []struct {
	path      string
	expectErr string
}{{
	path:      "x",
	expectErr: `need <user>/<name>`,
}, {
	path:      "/foo",
	expectErr: `invalid user name ""`,
}, {
	path:      "foo/",
	expectErr: `invalid name ""`,
}}

func (s *APISuite) TestNewModelWithInvalidControllerPath(c *gc.C) {
	for i, test := range newModelWithInvalidControllerPathTests {
		c.Logf("test %d", i)
		httptesting.AssertJSONCall(c, httptesting.JSONCallParams{
			Method:  "POST",
			URL:     "/v2/model/bob",
			Handler: s.JEMSrv,
			JSONBody: map[string]interface{}{
				"name":       "bar",
				"controller": test.path,
			},
			ExpectBody: params.Error{
				Message: fmt.Sprintf("cannot unmarshal parameters: cannot unmarshal into field: cannot unmarshal request body: %s", test.expectErr),
				Code:    params.ErrBadRequest,
			},
			ExpectStatus: http.StatusBadRequest,
			Do:           apitest.Do(s.IDMSrv.Client("bob")),
		})
	}
}

func (s *APISuite) TestNewModelCannotOpenAPI(c *gc.C) {
	s.AssertAddControllerDoc(c, &mongodoc.Controller{
		Path:      params.EntityPath{"bob", "foo"},
		AdminUser: "admin",
	})
	httptesting.AssertJSONCall(c, httptesting.JSONCallParams{
		Method:  "POST",
		URL:     "/v2/model/bob",
		Handler: s.JEMSrv,
		JSONBody: params.NewModelInfo{
			Name:       params.Name("bar"),
			Controller: &params.EntityPath{"bob", "foo"},
			Credential: "cred1",
		},
		ExpectBody: params.Error{
			Message: `cannot connect to controller: cannot connect to API: validating info for opening an API connection: missing addresses not valid`,
		},
		ExpectStatus: http.StatusInternalServerError,
		Do:           apitest.Do(s.IDMSrv.Client("bob")),
	})
}

func (s *APISuite) TestNewModelInvalidConfig(c *gc.C) {
	ctlId := s.AssertAddController(c, params.EntityPath{"bob", "foo"}, false)
	cred := s.AssertUpdateCredential(c, "bob", "dummy", "cred1", "empty")

	httptesting.AssertJSONCall(c, httptesting.JSONCallParams{
		Method:  "POST",
		URL:     "/v2/model/bob",
		Handler: s.JEMSrv,
		JSONBody: params.NewModelInfo{
			Name:       params.Name("bar"),
			Controller: &ctlId,
			Credential: cred,
			Config: map[string]interface{}{
				"authorized-keys": 123,
			},
		},
		ExpectBody: params.Error{
			Message: `cannot validate attributes: authorized-keys: expected string, got float64(123)`,
			Code:    params.ErrBadRequest,
		},
		ExpectStatus: http.StatusBadRequest,
		Do:           apitest.Do(s.IDMSrv.Client("bob")),
	})
}

func (s *APISuite) TestNewModelTwice(c *gc.C) {
	ctlId := s.AssertAddController(c, params.EntityPath{"bob", "foo"}, false)
	cred := s.AssertUpdateCredential(c, "bob", "dummy", "cred1", "empty")

	body := &params.NewModelInfo{
		Name:       "bar",
		Controller: &ctlId,
		Credential: cred,
		Config:     dummyModelConfig,
	}
	p := httptesting.JSONCallParams{
		Method:     "POST",
		URL:        "/v2/model/bob",
		Handler:    s.JEMSrv,
		JSONBody:   body,
		ExpectBody: apitest.AnyBody,
		Do:         apitest.Do(s.IDMSrv.Client("bob")),
	}
	httptesting.AssertJSONCall(c, p)

	// Creating the model the second time may fail because
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

func (s *APISuite) TestNewModelCannotCreate(c *gc.C) {
	ctlId := s.AssertAddController(c, params.EntityPath{"bob", "foo"}, false)
	cred := s.AssertUpdateCredential(c, "bob", "dummy", "cred1", "empty")

	httptesting.AssertJSONCall(c, httptesting.JSONCallParams{
		Method:  "POST",
		URL:     "/v2/model/bob",
		Handler: s.JEMSrv,
		JSONBody: params.NewModelInfo{
			Name:       "bar",
			Controller: &ctlId,
			Credential: cred,
			Config: map[string]interface{}{
				"authorized-keys": sshKey,
				"logging-config":  "bad>",
			},
		},
		ExpectBody: params.Error{
			Message: `cannot create model: failed to create config: creating config from values failed: config value expected '=', found "bad>"`,
		},
		ExpectStatus: http.StatusInternalServerError,
		Do:           apitest.Do(s.IDMSrv.Client("bob")),
	})

	// Check that the model is not there (it was added temporarily during the call).
	httptesting.AssertJSONCall(c, httptesting.JSONCallParams{
		Method:  "GET",
		Handler: s.JEMSrv,
		URL:     "/v2/model/bob/bar",
		ExpectBody: &params.Error{
			Message: `model "bob/bar" not found`,
			Code:    params.ErrNotFound,
		},
		ExpectStatus: http.StatusNotFound,
		Do:           apitest.Do(s.IDMSrv.Client("bob")),
	})
}

func (s *APISuite) TestNewModelUnauthorized(c *gc.C) {
	ctlId := s.AssertAddController(c, params.EntityPath{"bob", "foo"}, false)
	cred := s.AssertUpdateCredential(c, "bob", "dummy", "cred1", "empty")

	httptesting.AssertJSONCall(c, httptesting.JSONCallParams{
		Method:  "POST",
		URL:     "/v2/model/bob",
		Handler: s.JEMSrv,
		JSONBody: params.NewModelInfo{
			Name:       "bar",
			Controller: &ctlId,
			Credential: cred,
			Config:     dummyModelConfig,
		},
		ExpectBody: params.Error{
			Message: `unauthorized`,
			Code:    params.ErrUnauthorized,
		},
		ExpectStatus: http.StatusUnauthorized,
		Do:           apitest.Do(s.IDMSrv.Client("other")),
	})
}

func (s *APISuite) TestListController(c *gc.C) {
	ctlId0 := s.AssertAddController(c, params.EntityPath{"bob", "foo"}, true)

	ctlId1 := s.AssertAddController(c, params.EntityPath{"bob", "lost"}, false)
	unavailableTime := time.Now()
	err := s.JEM.DB.SetControllerUnavailableAt(ctlId1, unavailableTime)
	c.Assert(err, gc.IsNil)

	ctlId2 := s.AssertAddController(c, params.EntityPath{"bob", "another"}, false)
	err = s.JEM.DB.SetControllerUnavailableAt(ctlId2, unavailableTime.Add(time.Second))
	c.Assert(err, gc.IsNil)

	resp, err := s.NewClient("bob").ListController(nil)
	c.Assert(err, gc.IsNil)
	c.Assert(resp, jc.DeepEquals, &params.ListControllerResponse{
		Controllers: []params.ControllerResponse{{
			Path:             ctlId2,
			Location:         map[string]string{"cloud": "dummy", "region": "dummy-region"},
			UnavailableSince: newTime(mongodoc.Time(unavailableTime.Add(time.Second)).UTC()),
			Public:           true,
		}, {
			Path:     ctlId0,
			Location: map[string]string{"cloud": "dummy", "region": "dummy-region"},
			Public:   true,
		}, {
			Path:             ctlId1,
			Location:         map[string]string{"cloud": "dummy", "region": "dummy-region"},
			UnavailableSince: newTime(mongodoc.Time(unavailableTime).UTC()),
			Public:           true,
		}},
	})

	// Check that the private entries don't show up when listing
	// as a different user.
	resp, err = s.NewClient("alice").ListController(nil)
	c.Assert(err, gc.IsNil)
	c.Assert(resp, jc.DeepEquals, &params.ListControllerResponse{})
}

func (s *APISuite) TestListControllerNoServers(c *gc.C) {
	resp, err := s.NewClient("bob").ListController(nil)
	c.Assert(err, gc.IsNil)
	c.Assert(resp, jc.DeepEquals, &params.ListControllerResponse{})
}

func (s *APISuite) TestListModelsNoServers(c *gc.C) {
	resp, err := s.NewClient("bob").ListModels(nil)
	c.Assert(err, gc.IsNil)
	c.Assert(resp, jc.DeepEquals, &params.ListModelsResponse{})
}

func (s *APISuite) TestListModels(c *gc.C) {
	info := s.APIInfo(c)
	ctlId0 := s.AssertAddController(c, params.EntityPath{"alice", "foo"}, false)
	bCred := s.AssertUpdateCredential(c, "bob", "dummy", "cred1", "empty")
	cCred := s.AssertUpdateCredential(c, "charlie", "dummy", "cred1", "empty")
	s.allowModelPerm(c, ctlId0)
	s.allowControllerPerm(c, ctlId0)
	modelId1, uuid1 := s.CreateModel(c, params.EntityPath{"bob", "bar"}, ctlId0, bCred)
	modelId2, uuid2 := s.CreateModel(c, params.EntityPath{"charlie", "bar"}, ctlId0, cCred)
	err := s.JEM.DB.SetModelLife(ctlId0, uuid2, "alive")
	c.Assert(err, gc.IsNil)

	// Add an unavailable controller.
	ctlId1 := s.AssertAddController(c, params.EntityPath{"alice", "lost"}, false)
	c.Assert(err, gc.IsNil)
	s.allowModelPerm(c, ctlId1)
	s.allowControllerPerm(c, ctlId1)
	unavailableTime := time.Now()
	err = s.JEM.DB.SetControllerUnavailableAt(ctlId1, unavailableTime)
	c.Assert(err, gc.IsNil)

	resps := []params.ModelResponse{{
		Path:           ctlId0,
		UUID:           info.ModelTag.Id(),
		ControllerUUID: s.ControllerConfig.ControllerUUID(),
		CACert:         info.CACert,
		HostPorts:      info.Addrs,
		ControllerPath: ctlId0,
	}, {
		Path:           modelId1,
		UUID:           uuid1,
		ControllerUUID: s.ControllerConfig.ControllerUUID(),
		CACert:         info.CACert,
		HostPorts:      info.Addrs,
		ControllerPath: ctlId0,
	}, {
		Path:           modelId2,
		UUID:           uuid2,
		ControllerUUID: s.ControllerConfig.ControllerUUID(),
		CACert:         info.CACert,
		HostPorts:      info.Addrs,
		ControllerPath: ctlId0,
		Life:           "alive",
	}, {
		Path:             ctlId1,
		UUID:             info.ModelTag.Id(),
		ControllerUUID:   s.ControllerConfig.ControllerUUID(),
		CACert:           info.CACert,
		HostPorts:        info.Addrs,
		ControllerPath:   ctlId1,
		UnavailableSince: newTime(mongodoc.Time(unavailableTime).UTC()),
	}}
	tests := []struct {
		user    params.User
		indexes []int
	}{{
		user:    "bob",
		indexes: []int{0, 3, 1},
	}, {
		user:    "charlie",
		indexes: []int{0, 3, 2},
	}, {
		user:    "alice",
		indexes: []int{0, 3},
	}, {
		user:    "fred",
		indexes: []int{0, 3},
	}}
	for i, test := range tests {
		c.Logf("test %d: as user %s", i, test.user)
		expectResp := &params.ListModelsResponse{
			Models: make([]params.ModelResponse, len(test.indexes)),
		}
		for i, index := range test.indexes {
			expectResp.Models[i] = resps[index]
		}

		resp, err := s.NewClient(test.user).ListModels(nil)
		c.Assert(err, gc.IsNil)
		c.Assert(resp, jc.DeepEquals, expectResp)
	}
}

func (s *APISuite) TestGetSetControllerPerm(c *gc.C) {
	ctlId := s.AssertAddController(c, params.EntityPath{"alice", "foo"}, false)

	acl, err := s.NewClient("alice").GetControllerPerm(&params.GetControllerPerm{
		EntityPath: ctlId,
	})
	c.Assert(err, gc.IsNil)
	c.Assert(acl, jc.DeepEquals, params.ACL{})

	err = s.NewClient("alice").SetControllerPerm(&params.SetControllerPerm{
		EntityPath: ctlId,
		ACL: params.ACL{
			Read: []string{"a", "b"},
		},
	})
	c.Assert(err, gc.IsNil)
	acl, err = s.NewClient("alice").GetControllerPerm(&params.GetControllerPerm{
		EntityPath: ctlId,
	})
	c.Assert(err, gc.IsNil)
	c.Assert(acl, gc.DeepEquals, params.ACL{
		Read: []string{"a", "b"},
	})
}

func (s *APISuite) TestGetSetModelPerm(c *gc.C) {
	ctlId := s.AssertAddController(c, params.EntityPath{"alice", "foo"}, false)
	acl, err := s.NewClient("alice").GetModelPerm(&params.GetModelPerm{
		EntityPath: ctlId,
	})
	c.Assert(err, gc.IsNil)
	c.Assert(acl, jc.DeepEquals, params.ACL{})

	err = s.NewClient("alice").SetModelPerm(&params.SetModelPerm{
		EntityPath: ctlId,
		ACL: params.ACL{
			Read: []string{"a", "b"},
		},
	})
	c.Assert(err, gc.IsNil)
	acl, err = s.NewClient("alice").GetModelPerm(&params.GetModelPerm{
		EntityPath: ctlId,
	})
	c.Assert(err, gc.IsNil)
	c.Assert(acl, gc.DeepEquals, params.ACL{
		Read: []string{"a", "b"},
	})
}

func (s *APISuite) TestWhoAmI(c *gc.C) {
	resp, err := s.NewClient("bob").WhoAmI(nil)
	c.Assert(err, gc.IsNil)
	c.Assert(resp.User, gc.Equals, "bob")
}

func (s *APISuite) allowControllerPerm(c *gc.C, path params.EntityPath, acl ...string) {
	if len(acl) == 0 {
		acl = []string{"everyone"}
	}
	err := s.NewClient(path.User).SetControllerPerm(&params.SetControllerPerm{
		EntityPath: path,
		ACL: params.ACL{
			Read: acl,
		},
	})
	c.Assert(err, gc.IsNil)
}

func (s *APISuite) allowModelPerm(c *gc.C, path params.EntityPath, acl ...string) {
	if len(acl) == 0 {
		acl = []string{"everyone"}
	}
	err := s.NewClient(path.User).SetModelPerm(&params.SetModelPerm{
		EntityPath: path,
		ACL: params.ACL{
			Read: acl,
		},
	})
	c.Assert(err, gc.IsNil)
}
