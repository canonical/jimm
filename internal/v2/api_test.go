package v2_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/modelmanager"
	"github.com/juju/juju/api/usermanager"
	jujuparams "github.com/juju/juju/apiserver/params"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/testing/httptesting"
	gc "gopkg.in/check.v1"
	"gopkg.in/errgo.v1"

	"github.com/CanonicalLtd/jem/internal/apitest"
	"github.com/CanonicalLtd/jem/internal/mongodoc"
	"github.com/CanonicalLtd/jem/internal/v2"
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
	},
}, {
	about:  "new model with inaccessible controller",
	asUser: "alice",
	method: "POST",
	path:   "/v2/model/alice",
	body: params.NewModelInfo{
		Name:       "newmodel",
		Controller: &params.EntityPath{"bob", "private"},
	},
}, {
	about:  "new template as non-owner",
	asUser: "other",
	method: "PUT",
	path:   "/v2/template/bob/something",
	body: params.AddTemplateInfo{
		Controller: params.EntityPath{"bob", "open"},
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
	about:  "set template perm as non-owner",
	asUser: "other",
	method: "PUT",
	path:   "/v2/template/bob/open/perm",
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
	about:  "get template perm as non-owner",
	asUser: "other",
	method: "GET",
	path:   "/v2/template/bob/private/perm",
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
}, {
	about:  "get template perm with ACL that allows us",
	asUser: "other",
	method: "GET",
	path:   "/v2/template/bob/open/perm",
}}

func (s *APISuite) TestUnauthorized(c *gc.C) {
	s.assertAddController(c, params.EntityPath{"bob", "private"}, nil)
	ctlId := s.assertAddController(c, params.EntityPath{"bob", "open"}, nil)
	s.addTemplate(c, params.EntityPath{"bob", "open"}, ctlId, dummyModelConfig)
	s.addTemplate(c, params.EntityPath{"bob", "private"}, ctlId, dummyModelConfig)

	s.allowControllerPerm(c, params.EntityPath{"bob", "open"})
	s.allowModelPerm(c, params.EntityPath{"bob", "open"})
	s.allowTemplatePerm(c, params.EntityPath{"bob", "private"})

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
		about: "add model",
		body: params.ControllerInfo{
			HostPorts:      info.Addrs,
			CACert:         info.CACert,
			User:           info.Tag.Id(),
			Password:       info.Password,
			ControllerUUID: info.ModelTag.Id(),
		},
	}, {
		about:    "add model as part of group",
		username: "beatles",
		authUser: "alice",
		body: params.ControllerInfo{
			HostPorts:      info.Addrs,
			CACert:         info.CACert,
			User:           info.Tag.Id(),
			Password:       info.Password,
			ControllerUUID: info.ModelTag.Id(),
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
		},
		expectStatus: http.StatusBadRequest,
		expectBody: params.Error{
			Code:    "bad request",
			Message: "no host-ports in request",
		},
	}, {
		about: "no ca-cert",
		body: params.ControllerInfo{
			HostPorts:      info.Addrs,
			User:           info.Tag.Id(),
			Password:       info.Password,
			ControllerUUID: info.ModelTag.Id(),
		},
		expectStatus: http.StatusBadRequest,
		expectBody: params.Error{
			Code:    "bad request",
			Message: "no ca-cert in request",
		},
	}, {
		about: "no user",
		body: params.ControllerInfo{
			HostPorts:      info.Addrs,
			CACert:         info.CACert,
			Password:       info.Password,
			ControllerUUID: info.ModelTag.Id(),
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
		},
		expectStatus: http.StatusBadRequest,
		expectBody: params.Error{
			Code:    "bad request",
			Message: "bad model UUID in request",
		},
	}, {
		about: "cannot connect to evironment",
		body: params.ControllerInfo{
			HostPorts:      []string{"0.1.2.3:1234"},
			CACert:         info.CACert,
			User:           info.Tag.Id(),
			Password:       info.Password,
			ControllerUUID: info.ModelTag.Id(),
		},
		expectStatus: http.StatusBadRequest,
		expectBody: httptesting.BodyAsserter(func(c *gc.C, m json.RawMessage) {
			var body params.Error
			err := json.Unmarshal(m, &body)
			c.Assert(err, gc.IsNil)
			c.Assert(body.Code, gc.Equals, params.ErrBadRequest)
			c.Assert(body.Message, gc.Matches, `cannot connect to model: unable to connect to API: .*`)
		}),
	}}
	s.IDMSrv.AddUser("alice", "beatles")
	s.IDMSrv.AddUser("bob", "beatles")
	for i, test := range addControllerTests {
		c.Logf("test %d: %s", i, test.about)
		modelPath := params.EntityPath{
			User: test.username,
			Name: params.Name(fmt.Sprintf("model%d", i)),
		}
		if modelPath.User == "" {
			modelPath.User = "testuser"
		}
		authUser := test.authUser
		if authUser == "" {
			authUser = modelPath.User
		}
		client := s.IDMSrv.Client(string(authUser))
		httptesting.AssertJSONCall(c, httptesting.JSONCallParams{
			Method:       "PUT",
			Handler:      s.JEMSrv,
			JSONBody:     test.body,
			URL:          fmt.Sprintf("/v2/controller/%s", modelPath),
			Do:           apitest.Do(client),
			ExpectStatus: test.expectStatus,
			ExpectBody:   test.expectBody,
		})
		if test.expectStatus != 0 {
			continue
		}
		// The controller was added successfully. Check that we
		// can fetch its associated model and that we
		// can connect to that.
		modelResp, err := s.NewClient(authUser).GetModel(&params.GetModel{
			EntityPath: modelPath,
		})
		c.Assert(err, gc.IsNil)
		st := openAPIFromModelResponse(c, modelResp)
		st.Close()
		c.Assert(modelResp.Password, gc.Not(gc.Equals), "")
		modelResp.Password = ""
		c.Assert(modelResp, jc.DeepEquals, &params.ModelResponse{
			Path:           modelPath,
			User:           "jem-" + string(authUser),
			HostPorts:      test.body.HostPorts,
			CACert:         test.body.CACert,
			UUID:           test.body.ControllerUUID,
			ControllerUUID: test.body.ControllerUUID,
			ControllerPath: modelPath,
		})
		// Clear the connection pool for the next test.
		s.JEMSrv.Pool().ClearAPIConnCache()
	}
}

func (s *APISuite) TestAddControllerDuplicate(c *gc.C) {
	ctlPath := s.assertAddController(c, params.EntityPath{"bob", "dupmodel"}, nil)
	err := s.addController(c, ctlPath, nil)
	c.Assert(err, gc.ErrorMatches, "PUT http://.*: already exists")
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
	ctlId := s.assertAddController(c, params.EntityPath{"bob", "who"}, nil)
	modelPath := params.EntityPath{"bob", "foobarred"}
	modelId, user, uuid := s.addModel(c, modelPath, ctlId)
	resp := httptesting.DoRequest(c, httptesting.DoRequestParams{
		Handler: s.JEMSrv,
		URL:     "/v2/model/" + modelId.String(),
		Do:      apitest.Do(s.IDMSrv.Client("bob")),
	})
	c.Assert(resp.Code, gc.Equals, http.StatusOK, gc.Commentf("body: %s", resp.Body.Bytes()))
	c.Assert(user, gc.NotNil)
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

	// Try deleting model for Controller.
	httptesting.AssertJSONCall(c, httptesting.JSONCallParams{
		Method:       "DELETE",
		Handler:      s.JEMSrv,
		URL:          "/v2/model/" + ctlId.String(),
		ExpectStatus: http.StatusForbidden,
		ExpectBody: &params.Error{
			Message: `cannot remove model "bob/who" because it is a controller`,
			Code:    params.ErrForbidden,
		},
		Do: apitest.Do(s.IDMSrv.Client("bob")),
	})
}

func (s *APISuite) TestGetController(c *gc.C) {
	ctlId := s.assertAddController(c, params.EntityPath{"bob", "foo"}, nil)

	resp := httptesting.DoRequest(c, httptesting.DoRequestParams{
		Handler: s.JEMSrv,
		URL:     "/v2/controller/" + ctlId.String(),
		Do:      apitest.Do(s.IDMSrv.Client("bob")),
	})
	c.Assert(resp.Code, gc.Equals, http.StatusOK, gc.Commentf("body: %s", resp.Body.Bytes()))
	var controllerInfo params.ControllerResponse
	err := json.Unmarshal(resp.Body.Bytes(), &controllerInfo)
	c.Assert(err, gc.IsNil, gc.Commentf("body: %s", resp.Body.String()))
	c.Assert(controllerInfo.ProviderType, gc.Equals, "dummy")
	c.Assert(controllerInfo.Schema, gc.Not(gc.HasLen), 0)
	// Check that all path attributes have been removed.
	for name := range controllerInfo.Schema {
		c.Assert(strings.HasSuffix(name, "-path"), gc.Equals, false)
	}
	c.Logf("%#v", controllerInfo.Schema)
}

func (s *APISuite) TestGetControllerWithLocation(c *gc.C) {
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
			Location:       map[string]string{"cloud": "aws"},
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
	c.Assert(controllerInfo.Location, gc.DeepEquals, map[string]string{"cloud": "aws"})
}

func (s *APISuite) TestGetControllerLocation(c *gc.C) {
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
			Location:       map[string]string{"cloud": "aws"},
		},
	})
	c.Assert(err, gc.IsNil)

	// Check the location attributes.
	httptesting.AssertJSONCall(c, httptesting.JSONCallParams{
		Method:       "GET",
		Handler:      s.JEMSrv,
		URL:          "/v2/controller/" + ctlId.String() + "/meta/location",
		ExpectStatus: http.StatusOK,
		ExpectBody: params.ControllerLocation{
			Location: map[string]string{"cloud": "aws"},
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

func (s *APISuite) TestSetControllerLocation(c *gc.C) {
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
		},
	})
	c.Assert(err, gc.IsNil)

	// Check there is no location attributes.
	httptesting.AssertJSONCall(c, httptesting.JSONCallParams{
		Method:       "GET",
		Handler:      s.JEMSrv,
		URL:          "/v2/controller/" + ctlId.String() + "/meta/location",
		ExpectStatus: http.StatusOK,
		ExpectBody:   params.ControllerLocation{Location: map[string]string{}},
		Do:           apitest.Do(s.IDMSrv.Client("bob")),
	})

	// Put some location attributes.
	resp := httptesting.DoRequest(c, httptesting.DoRequestParams{
		Method:  "PUT",
		Handler: s.JEMSrv,
		URL:     "/v2/controller/" + ctlId.String() + "/meta/location",
		Do:      apitest.Do(s.IDMSrv.Client("bob")),
		JSONBody: params.ControllerLocation{
			Location: map[string]string{"cloud": "aws"},
		},
	})

	c.Assert(resp.Code, gc.Equals, http.StatusOK, gc.Commentf("body: %s", resp.Body.Bytes()))

	// Retrieve the newly put location attributes.
	httptesting.AssertJSONCall(c, httptesting.JSONCallParams{
		Method:       "GET",
		Handler:      s.JEMSrv,
		URL:          "/v2/controller/" + ctlId.String() + "/meta/location",
		ExpectStatus: http.StatusOK,
		ExpectBody: params.ControllerLocation{
			Location: map[string]string{"cloud": "aws"},
		},
		Do: apitest.Do(s.IDMSrv.Client("bob")),
	})

	// Check that alice can't modify it.
	resp = httptesting.DoRequest(c, httptesting.DoRequestParams{
		Method:   "PUT",
		Handler:  s.JEMSrv,
		URL:      "/v2/controller/" + ctlId.String() + "/meta/location",
		Do:       apitest.Do(s.IDMSrv.Client("alice")),
		JSONBody: map[string]string{"cloud": "aws"},
	})

	c.Assert(resp.Code, gc.Equals, http.StatusUnauthorized, gc.Commentf("body: %s", resp.Body.Bytes()))

	// Check that alice can't probe controller with PUT.
	resp = httptesting.DoRequest(c, httptesting.DoRequestParams{
		Method:   "PUT",
		Handler:  s.JEMSrv,
		URL:      "/v2/controller/bob/unknown/meta/location",
		Do:       apitest.Do(s.IDMSrv.Client("alice")),
		JSONBody: map[string]string{"cloud": "aws"},
	})

	c.Assert(resp.Code, gc.Equals, http.StatusUnauthorized, gc.Commentf("body: %s", resp.Body.Bytes()))
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
	// Define controller.
	ctlId := s.assertAddController(c, params.EntityPath{"bob", "foobarred"}, nil)
	// Add controller to JEM.
	resp := httptesting.DoRequest(c, httptesting.DoRequestParams{
		Handler: s.JEMSrv,
		URL:     "/v2/controller/" + ctlId.String(),
		Do:      apitest.Do(s.IDMSrv.Client("bob")),
	})
	// Assert that it was added.
	c.Assert(resp.Code, gc.Equals, http.StatusOK, gc.Commentf("body: %s", resp.Body.Bytes()))
	// Add another model to it.
	modelId, _, _ := s.addModel(c, params.EntityPath{"bob", "bar"}, ctlId)
	// Delete controller.
	httptesting.AssertJSONCall(c, httptesting.JSONCallParams{
		Method:       "DELETE",
		Handler:      s.JEMSrv,
		URL:          "/v2/controller/bob/foobarred",
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
	about: "multiple filters",
	user:  "bob",
	arg: params.GetControllerLocations{
		Attr: "region",
		Location: map[string]string{
			"cloud":   "aws",
			"staging": "true",
		},
	},
	expect: params.ControllerLocationsResponse{
		Values: []string{"us-east-1"},
	},
}, {
	about: "no matching controllers",
	user:  "bob",
	arg: params.GetControllerLocations{
		Attr: "region",
		Location: map[string]string{
			"cloud":         "aws",
			"somethingelse": "blah",
		},
	},
	expect: params.ControllerLocationsResponse{},
}, {
	about: "no controllers with attribute",
	user:  "bob",
	arg: params.GetControllerLocations{
		Attr: "region",
		Location: map[string]string{
			"somethingelse": "blah",
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
			"staging":    "true",
		},
	},
	expectError: `GET .*: invalid location attribute "cloud\.blah"`,
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
	s.assertAddController(c, params.EntityPath{"bob", "aws-us-east"}, map[string]string{
		"cloud":  "aws",
		"region": "us-east-1",
	})
	s.assertAddController(c, params.EntityPath{"bob", "aws-us-east-staging"}, map[string]string{
		"cloud":   "aws",
		"region":  "us-east-1",
		"staging": "true",
	})
	s.assertAddController(c, params.EntityPath{"bob", "aws-eu-west"}, map[string]string{
		"cloud":  "aws",
		"region": "eu-west-1",
	})
	s.assertAddController(c, params.EntityPath{"bob", "gce-somewhere"}, map[string]string{
		"cloud":  "gce",
		"region": "somewhere",
	})
	s.assertAddController(c, params.EntityPath{"bob", "gce-somewhere-staging"}, map[string]string{
		"cloud":   "gce",
		"region":  "somewhere",
		"staging": "true",
	})
	s.allowControllerPerm(c, params.EntityPath{"bob", "gce-somewhere-staging"}, "somegroup")
	s.assertAddController(c, params.EntityPath{"bob", "gce-elsewhere"}, map[string]string{
		"cloud":  "gce",
		"region": "elsewhere",
	})
	s.assertAddController(c, params.EntityPath{"alice", "alice-controller"}, map[string]string{
		"cloud":  "azure",
		"region": "america",
	})

	s.IDMSrv.AddUser("alice", "somegroup")

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
			"cloud":   "aws",
			"region":  "us-east-1",
			"staging": "true",
		}, {
			"cloud":  "aws",
			"region": "us-east-1",
		}, {
			"cloud":  "gce",
			"region": "elsewhere",
		}, {
			"cloud":   "gce",
			"region":  "somewhere",
			"staging": "true",
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
			"cloud":   "aws",
			"region":  "us-east-1",
			"staging": "true",
		}, {
			"cloud":  "aws",
			"region": "us-east-1",
		}},
	},
}, {
	about: "multiple filters",
	user:  "bob",
	arg: params.GetAllControllerLocations{
		Location: map[string]string{
			"cloud":   "aws",
			"staging": "true",
		},
	},
	expect: params.AllControllerLocationsResponse{
		Locations: []map[string]string{{
			"cloud":   "aws",
			"region":  "us-east-1",
			"staging": "true",
		}},
	},
}, {
	about: "no matching controllers",
	user:  "bob",
	arg: params.GetAllControllerLocations{
		Location: map[string]string{
			"cloud":         "aws",
			"somethingelse": "blah",
		},
	},
	expect: params.AllControllerLocationsResponse{},
}, {
	about: "no controllers with attribute",
	user:  "bob",
	arg: params.GetAllControllerLocations{
		Location: map[string]string{
			"somethingelse": "blah",
		},
	},
	expect: params.AllControllerLocationsResponse{},
}, {
	about: "invalid filter attribute",
	user:  "bob",
	arg: params.GetAllControllerLocations{
		Location: map[string]string{
			"cloud.blah": "aws",
			"staging":    "true",
		},
	},
	expectError: `GET .*: invalid location attribute "cloud\.blah"`,
	expectCause: params.ErrBadRequest,
}, {
	about: "user without access to everything",
	user:  "alice",
	expect: params.AllControllerLocationsResponse{
		Locations: []map[string]string{{
			"cloud":  "azure",
			"region": "america",
		}, {
			"cloud":   "gce",
			"region":  "somewhere",
			"staging": "true",
		}},
	},
}}

func (s *APISuite) TestAllControllerLocations(c *gc.C) {
	s.assertAddController(c, params.EntityPath{"bob", "aws-us-east"}, map[string]string{
		"cloud":  "aws",
		"region": "us-east-1",
	})
	s.assertAddController(c, params.EntityPath{"bob", "aws-us-east-2"}, map[string]string{
		"cloud":  "aws",
		"region": "us-east-1",
	})
	s.assertAddController(c, params.EntityPath{"bob", "aws-us-east-staging"}, map[string]string{
		"cloud":   "aws",
		"region":  "us-east-1",
		"staging": "true",
	})
	s.assertAddController(c, params.EntityPath{"bob", "aws-us-east-staging-2"}, map[string]string{
		"cloud":   "aws",
		"region":  "us-east-1",
		"staging": "true",
	})
	s.assertAddController(c, params.EntityPath{"bob", "aws-eu-west"}, map[string]string{
		"cloud":  "aws",
		"region": "eu-west-1",
	})
	s.assertAddController(c, params.EntityPath{"bob", "gce-somewhere"}, map[string]string{
		"cloud":  "gce",
		"region": "somewhere",
	})
	s.assertAddController(c, params.EntityPath{"bob", "gce-somewhere-staging"}, map[string]string{
		"cloud":   "gce",
		"region":  "somewhere",
		"staging": "true",
	})
	s.allowControllerPerm(c, params.EntityPath{"bob", "gce-somewhere-staging"}, "somegroup")
	s.assertAddController(c, params.EntityPath{"bob", "gce-elsewhere"}, map[string]string{
		"cloud":  "gce",
		"region": "elsewhere",
	})
	s.assertAddController(c, params.EntityPath{"alice", "alice-controller"}, map[string]string{
		"cloud":  "azure",
		"region": "america",
	})
	s.assertAddController(c, params.EntityPath{"alice", "forgotten"}, nil)

	s.IDMSrv.AddUser("alice", "somegroup")

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
	ctlId := s.assertAddController(c, params.EntityPath{"bob", "aws-us-east"}, map[string]string{
		"cloud":  "aws",
		"region": "us-east-1",
	})
	s.assertAddController(c, params.EntityPath{"bob", "aws-eu-west"}, map[string]string{
		"cloud":  "aws",
		"region": "eu-west-1",
	})
	ctl, err := s.NewClient("bob").GetController(&params.GetController{
		EntityPath: ctlId,
	})
	c.Assert(err, gc.IsNil)
	resp, err := s.NewClient("bob").GetSchema(&params.GetSchema{
		Location: map[string]string{
			"cloud": "aws",
		},
	})
	c.Assert(err, gc.IsNil)

	c.Assert(resp.ProviderType, gc.Equals, ctl.ProviderType)
	c.Assert(resp.Schema, jc.DeepEquals, ctl.Schema)
}

func (s *APISuite) TestGetSchemaNotFound(c *gc.C) {
	s.assertAddController(c, params.EntityPath{"bob", "aws-us-east"}, map[string]string{
		"cloud":  "aws",
		"region": "us-east-1",
	})
	resp, err := s.NewClient("bob").GetSchema(&params.GetSchema{
		Location: map[string]string{
			"cloud": "ec2",
		},
	})
	c.Check(resp, gc.IsNil)
	c.Check(errgo.Cause(err), gc.Equals, params.ErrNotFound)
	c.Assert(err, gc.ErrorMatches, `GET http://.*/schema\?cloud=ec2: no matching controllers`)
}

func (s *APISuite) TestGetSchemaAmbiguous(c *gc.C) {
	s.assertAddController(c, params.EntityPath{"bob", "aws-us-east"}, map[string]string{
		"cloud":  "aws",
		"region": "us-east-1",
	})
	// Add a controller directly to the database because we can only
	// have dummy provider controllers otherwise and we need one
	// of a different type.
	err := s.JEM.AddController(&mongodoc.Controller{
		Path: params.EntityPath{"bob", "azure"},
		UUID: "fake-uuid",
		Location: map[string]string{
			"cloud": "aws",
		},
		ProviderType: "another",
	}, &mongodoc.Model{
		Path:       params.EntityPath{"bob", "azure"},
		Controller: params.EntityPath{"bob", "azure"},
	})
	c.Assert(err, gc.IsNil)

	resp, err := s.NewClient("bob").GetSchema(&params.GetSchema{
		Location: map[string]string{
			"cloud": "aws",
		},
	})
	c.Check(resp, gc.IsNil)
	c.Check(errgo.Cause(err), gc.Equals, params.ErrAmbiguousLocation)
	c.Assert(err, gc.ErrorMatches, `GET http://.*/schema\?cloud=aws: ambiguous location matches controller of more than one type`)
}

func (s *APISuite) TestNewModel(c *gc.C) {
	ctlId := s.assertAddController(c, params.EntityPath{"bob", "foo"}, nil)

	var modelRespBody json.RawMessage
	httptesting.AssertJSONCall(c, httptesting.JSONCallParams{
		Method:  "POST",
		URL:     "/v2/model/bob",
		Handler: s.JEMSrv,
		JSONBody: params.NewModelInfo{
			Name:       params.Name("bar"),
			Controller: &ctlId,
			Config:     dummyModelConfig,
		},
		ExpectBody: httptesting.BodyAsserter(func(_ *gc.C, body json.RawMessage) {
			modelRespBody = body
		}),
		Do: apitest.Do(s.IDMSrv.Client("bob")),
	})
	var modelResp params.ModelResponse
	err := json.Unmarshal(modelRespBody, &modelResp)
	c.Assert(err, gc.IsNil)

	st := openAPIFromModelResponse(c, &modelResp)
	defer st.Close()

	minfo, err := st.Client().ModelInfo()
	c.Assert(err, gc.IsNil)
	c.Assert(minfo.UUID, gc.Not(gc.Equals), "")
	c.Assert(minfo.UUID, gc.Not(gc.Equals), s.APIInfo(c).ModelTag.Id())
	c.Assert(minfo.ControllerUUID, gc.Equals, s.APIInfo(c).ModelTag.Id())

	// Ensure that we can connect to the new model
	// from the information returned by GetModel.
	modelResp2, err := s.NewClient("bob").GetModel(&params.GetModel{
		EntityPath: params.EntityPath{
			User: "bob",
			Name: "bar",
		},
	})
	c.Assert(err, gc.IsNil)
	st = openAPIFromModelResponse(c, modelResp2)
	defer st.Close()
	minfo2, err := st.Client().ModelInfo()
	c.Assert(err, gc.IsNil)
	c.Assert(minfo2.UUID, gc.Equals, minfo.UUID)
	c.Assert(minfo2.ControllerUUID, gc.Equals, minfo.ControllerUUID)
}

func (s *APISuite) TestNewModelWithTemplates(c *gc.C) {
	ctlId := s.assertAddController(c, params.EntityPath{"bob", "foo"}, nil)

	// TODO change "admin-secret" to "secret" when we can
	// make the "secret" configuration attribute marked as secret
	// in the schema.
	s.addTemplate(c, params.EntityPath{"bob", "creds"}, ctlId, map[string]interface{}{
		"secret":          "my secret",
		"authorized-keys": sshKey,
		"broken":          "something",
		"controller":      false,
	})
	s.addTemplate(c, params.EntityPath{"bob", "other"}, ctlId, map[string]interface{}{
		"broken": "nothing",
	})

	modelPath := params.EntityPath{"bob", "model"}
	resp, err := s.NewClient("bob").NewModel(&params.NewModel{
		User: modelPath.User,
		Info: params.NewModelInfo{
			Name:       modelPath.Name,
			Controller: &ctlId,
			Config: map[string]interface{}{
				"secret": "another secret",
			},
			TemplatePaths: []params.EntityPath{{"bob", "creds"}, {"bob", "other"}},
		},
	})
	c.Assert(err, gc.IsNil)

	// Check that the model was actually created with the right
	// configuration.
	m, err := s.State.GetModel(names.NewModelTag(resp.UUID))
	c.Assert(err, gc.IsNil)
	cfg, err := m.Config()
	c.Assert(err, gc.IsNil)
	attrs := cfg.AllAttrs()
	c.Logf("all attributes for uuid %v: %#v", resp.UUID, attrs)
	c.Assert(attrs["secret"], gc.Equals, "another secret")
	c.Assert(attrs["controller"], gc.Equals, false)
	c.Assert(attrs["broken"], gc.Equals, "nothing")
	c.Assert(attrs["authorized-keys"], gc.Equals, sshKey)

	st := openAPIFromModelResponse(c, resp)
	st.Close()
}

func (s *APISuite) TestNewModelWithTemplateNotFound(c *gc.C) {
	ctlId := s.assertAddController(c, params.EntityPath{"bob", "foo"}, nil)

	resp, err := s.NewClient("bob").NewModel(&params.NewModel{
		User: "bob",
		Info: params.NewModelInfo{
			Name:       "x",
			Controller: &ctlId,
			Config: map[string]interface{}{
				"secret": "another secret",
			},
			TemplatePaths: []params.EntityPath{{"bob", "creds"}},
		},
	})
	c.Assert(err, gc.ErrorMatches, `POST .*/v2/model/bob: cannot get template "bob/creds": template "bob/creds" not found`)
	c.Assert(resp, gc.IsNil)
}

func (s *APISuite) TestNewModelWithLocation(c *gc.C) {
	var nextIndex int
	s.PatchValue(v2.RandIntn, func(n int) int {
		return nextIndex % n
	})
	// Note: the controllers are ordered alphabetically by id before
	// the "random" selection is applied.
	s.assertAddController(c, params.EntityPath{"bob", "aws-eu-west"}, map[string]string{
		"cloud":  "aws",
		"region": "eu-west-1",
	})
	s.assertAddController(c, params.EntityPath{"bob", "aws-us-east"}, map[string]string{
		"cloud":  "aws",
		"region": "us-east-1",
	})
	s.assertAddController(c, params.EntityPath{"bob", "aws-us-east-staging"}, map[string]string{
		"cloud":   "aws",
		"region":  "us-east-1",
		"staging": "true",
	})
	s.allowControllerPerm(c, params.EntityPath{"bob", "aws-us-east-staging"}, "everyone")
	s.assertAddController(c, params.EntityPath{"bob", "azure-us"}, map[string]string{
		"cloud":  "azure",
		"region": "us",
	})

	tests := []struct {
		about            string
		randIndex        int
		user             params.User
		arg              params.NewModel
		expectController string
		expectError      string
		expectCause      error
	}{{
		about:     "select aws",
		randIndex: 1,
		user:      "bob",
		arg: params.NewModel{
			User: "bob",
			Info: params.NewModelInfo{
				Location: map[string]string{
					"cloud": "aws",
				},
				Config: map[string]interface{}{
					"secret": "a secret",
				},
			},
		},
		expectController: "bob/aws-us-east",
	}, {
		about:     "select aws; index 0",
		randIndex: 0,
		user:      "bob",
		arg: params.NewModel{
			User: "bob",
			Info: params.NewModelInfo{
				Location: map[string]string{
					"cloud": "aws",
				},
				Config: map[string]interface{}{
					"secret": "a secret",
				},
			},
		},
		expectController: "bob/aws-eu-west",
	}, {
		about:     "select staging",
		randIndex: 0,
		user:      "bob",
		arg: params.NewModel{
			User: "bob",
			Info: params.NewModelInfo{
				Location: map[string]string{
					"staging": "true",
				},
				Config: map[string]interface{}{
					"secret": "a secret",
				},
			},
		},
		expectController: "bob/aws-us-east-staging",
	}, {
		about:     "some other user only gets to see the publicly available controller",
		randIndex: 0,
		user:      "alice",
		arg: params.NewModel{
			User: "alice",
			Info: params.NewModelInfo{
				Config: map[string]interface{}{
					"secret": "a secret",
				},
			},
		},
		expectController: "bob/aws-us-east-staging",
	}, {
		about: "no matching controller",
		user:  "alice",
		arg: params.NewModel{
			User: "alice",
			Info: params.NewModelInfo{
				Location: map[string]string{
					"cloud": "somewhere",
				},
				Config: map[string]interface{}{
					"secret": "a secret",
				},
			},
		},
		expectError: `POST .*: cannot select controller: no matching controllers found`,
		expectCause: params.ErrNotFound,
	}, {
		about: "bad location attribute",
		user:  "bob",
		arg: params.NewModel{
			User: "bob",
			Info: params.NewModelInfo{
				Location: map[string]string{
					"$wrong": "somewhere",
				},
				Config: map[string]interface{}{
					"secret": "a secret",
				},
			},
		},
		expectError: `POST http://.*/v2/model/bob: cannot select controller: bad controller location query: invalid attribute "\$wrong"`,
		expectCause: params.ErrBadRequest,
	}}
	for i, test := range tests {
		c.Logf("test %d: %d", i, test.about)
		nextIndex = test.randIndex
		test.arg.Info.Name = params.Name(fmt.Sprintf("x%d", i))
		resp, err := s.NewClient(test.user).NewModel(&test.arg)
		if test.expectError != "" {
			c.Check(errgo.Cause(err), gc.Equals, test.expectCause)
			c.Assert(err, gc.ErrorMatches, test.expectError)
			continue
		}
		c.Assert(err, gc.IsNil)
		c.Assert(resp.Path, jc.DeepEquals, params.EntityPath{test.arg.User, test.arg.Info.Name})
		c.Assert(resp.ControllerPath.String(), gc.Equals, test.expectController)
	}
}

func (s *APISuite) TestGetModelWithExplicitlyRemovedUser(c *gc.C) {
	ctlId := s.assertAddController(c, params.EntityPath{"bob", "foo"}, nil)

	s.IDMSrv.AddUser("alice", "buddies")
	s.IDMSrv.AddUser("bob", "buddies")
	resp, err := s.NewClient("bob").NewModel(&params.NewModel{
		User: "buddies",
		Info: params.NewModelInfo{
			Name:       "bobsmodel",
			Controller: &ctlId,
			Config:     dummyModelConfig,
		},
	})
	c.Assert(err, gc.IsNil)
	modelId := resp.Path

	// Alice is part of the same group, so check that she can get and use
	// the model.
	resp, err = s.NewClient("alice").GetModel(&params.GetModel{
		EntityPath: modelId,
	})
	c.Assert(err, gc.IsNil)
	st := openAPIFromModelResponse(c, resp)
	st.Close()

	// Explicitly revoke access directly on the controller.
	mmClient := modelmanager.NewClient(s.APIState)
	err = mmClient.RevokeModel("jem-alice", string(jujuparams.ModelReadAccess), resp.UUID)
	c.Assert(err, gc.IsNil)

	// Sanity-check that we really have removed access.
	st, err = api.Open(apiInfoFromModelResponse(resp), api.DialOpts{})
	c.Assert(err, jc.Satisfies, jujuparams.IsCodeUnauthorized)

	// Get the model again. Alice should not be added back to the permissions.
	resp, err = s.NewClient("alice").GetModel(&params.GetModel{
		EntityPath: modelId,
	})
	c.Assert(err, gc.IsNil)

	// Alice should still be denied, despite having access to the model
	// in JEM.
	st, err = api.Open(apiInfoFromModelResponse(resp), api.DialOpts{})
	c.Assert(err, gc.NotNil)
	c.Assert(err, jc.Satisfies, jujuparams.IsCodeUnauthorized)
}

func openAPIFromModelResponse(c *gc.C, resp *params.ModelResponse) api.Connection {
	st, err := api.Open(apiInfoFromModelResponse(resp), api.DialOpts{})
	c.Assert(err, gc.IsNil, gc.Commentf("user: %q; password: %q", resp.User, resp.Password))
	return st
}

func apiInfoFromModelResponse(resp *params.ModelResponse) *api.Info {
	return &api.Info{
		Tag:      names.NewUserTag(resp.User),
		Password: resp.Password,
		Addrs:    resp.HostPorts,
		CACert:   resp.CACert,
		ModelTag: names.NewModelTag(resp.UUID),
	}
}

func (s *APISuite) TestNewModelUnderGroup(c *gc.C) {
	ctlId := s.assertAddController(c, params.EntityPath{"bob", "foo"}, nil)

	s.IDMSrv.AddUser("bob", "beatles")
	var modelRespBody json.RawMessage
	httptesting.AssertJSONCall(c, httptesting.JSONCallParams{
		Method:  "POST",
		URL:     "/v2/model/beatles",
		Handler: s.JEMSrv,
		JSONBody: params.NewModelInfo{
			Name:       params.Name("bar"),
			Controller: &ctlId,
			Config:     dummyModelConfig,
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
	st := openAPIFromModelResponse(c, &modelResp)
	st.Close()
}

func (s *APISuite) TestNewModelWithExistingUser(c *gc.C) {
	username := "jem-bob"

	_, _, err := usermanager.NewClient(s.APIState).AddUser(username, "", "old", "")
	c.Assert(err, gc.IsNil)

	ctlId := s.assertAddController(c, params.EntityPath{"bob", "foo"}, nil)

	var modelRespBody json.RawMessage
	httptesting.AssertJSONCall(c, httptesting.JSONCallParams{
		Method:  "POST",
		URL:     "/v2/model/bob",
		Handler: s.JEMSrv,
		JSONBody: params.NewModelInfo{
			Name:       params.Name("bar"),
			Controller: &ctlId,
			Config:     dummyModelConfig,
		},
		ExpectBody: httptesting.BodyAsserter(func(_ *gc.C, body json.RawMessage) {
			modelRespBody = body
		}),
		Do: apitest.Do(s.IDMSrv.Client("bob")),
	})
	var modelResp params.ModelResponse
	err = json.Unmarshal(modelRespBody, &modelResp)
	c.Assert(err, gc.IsNil)

	// Make sure that we really are reusing the username.
	c.Assert(modelResp.User, gc.Equals, username)

	// Ensure that we can connect to the new model with
	// the new secret
	apiInfo := &api.Info{
		Tag:      names.NewUserTag(username),
		Password: modelResp.Password,
		Addrs:    modelResp.HostPorts,
		CACert:   modelResp.CACert,
		ModelTag: names.NewModelTag(modelResp.UUID),
	}
	st, err := api.Open(apiInfo, api.DialOpts{})
	c.Assert(err, gc.IsNil)
	defer st.Close()
}

var newModelWithInvalidControllerPathTests = []struct {
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
	httptesting.AssertJSONCall(c, httptesting.JSONCallParams{
		Method:  "POST",
		URL:     "/v2/model/bob",
		Handler: s.JEMSrv,
		JSONBody: params.NewModelInfo{
			Name:       params.Name("bar"),
			Controller: &params.EntityPath{"bob", "foo"},
		},
		ExpectBody: params.Error{
			Message: `cannot connect to controller: cannot get model: model "bob/foo" not found`,
			Code:    params.ErrNotFound,
		},
		ExpectStatus: http.StatusNotFound,
		Do:           apitest.Do(s.IDMSrv.Client("bob")),
	})
}

func (s *APISuite) TestNewModelInvalidConfig(c *gc.C) {
	ctlId := s.assertAddController(c, params.EntityPath{"bob", "foo"}, nil)

	httptesting.AssertJSONCall(c, httptesting.JSONCallParams{
		Method:  "POST",
		URL:     "/v2/model/bob",
		Handler: s.JEMSrv,
		JSONBody: params.NewModelInfo{
			Name:       params.Name("bar"),
			Controller: &ctlId,
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
	ctlId := s.assertAddController(c, params.EntityPath{"bob", "foo"}, nil)

	body := &params.NewModelInfo{
		Name:       "bar",
		Controller: &ctlId,
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
	ctlId := s.assertAddController(c, params.EntityPath{"bob", "foo"}, nil)

	httptesting.AssertJSONCall(c, httptesting.JSONCallParams{
		Method:  "POST",
		URL:     "/v2/model/bob",
		Handler: s.JEMSrv,
		JSONBody: params.NewModelInfo{
			Name:       "bar",
			Controller: &ctlId,
			Config: map[string]interface{}{
				"authorized-keys": sshKey,
				"logging-config":  "bad>",
			},
		},
		ExpectBody: params.Error{
			Message: `cannot create model: failed to create config: creating config from values failed: logger specification expected '=', found "bad>"`,
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
	ctlId := s.assertAddController(c, params.EntityPath{"bob", "foo"}, nil)

	httptesting.AssertJSONCall(c, httptesting.JSONCallParams{
		Method:  "POST",
		URL:     "/v2/model/bob",
		Handler: s.JEMSrv,
		JSONBody: params.NewModelInfo{
			Name:       "bar",
			Controller: &ctlId,
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
	ctlId := s.assertAddController(c, params.EntityPath{"bob", "foo"}, nil)
	resp, err := s.NewClient("bob").ListController(nil)
	c.Assert(err, gc.IsNil)
	c.Assert(resp, jc.DeepEquals, &params.ListControllerResponse{
		Controllers: []params.ControllerResponse{{
			Path:     ctlId,
			Location: map[string]string{},
		}},
	})

	// Check that the entry doesn't show up when listing
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

func (s *APISuite) TestListTemplates(c *gc.C) {
	ctlId := s.assertAddController(c, params.EntityPath{"bob", "foo"}, nil)
	s.addTemplate(c, params.EntityPath{"bob", "other"}, ctlId, map[string]interface{}{
		"controller": true,
	})
	s.addTemplate(c, params.EntityPath{"bob", "creds"}, ctlId, map[string]interface{}{
		"secret":          "my secret",
		"authorized-keys": sshKey,
		"controller":      false,
	})
	s.addTemplate(c, params.EntityPath{"alice", "x"}, ctlId, map[string]interface{}{
		"controller":      false,
		"authorized-keys": sshKey,
	})
	s.addTemplate(c, params.EntityPath{"alice", "y"}, ctlId, map[string]interface{}{
		"controller": false,
	})
	s.allowTemplatePerm(c, params.EntityPath{"alice", "y"})

	tmpl0, err := s.NewClient("bob").GetTemplate(&params.GetTemplate{
		EntityPath: params.EntityPath{"bob", "creds"},
	})
	c.Assert(err, gc.IsNil)

	resp, err := s.NewClient("bob").ListTemplates(&params.ListTemplates{})
	c.Assert(err, gc.IsNil)

	// checkAndClear checks the schemas in the templates
	// response and clears them (they're all the same)
	// so we can DeepEquals the rest.
	checkAndClear := func(resp *params.ListTemplatesResponse) {
		for i := range resp.Templates {
			tmpl := &resp.Templates[i]
			c.Assert(tmpl.Schema, jc.DeepEquals, tmpl0.Schema)
			tmpl.Schema = nil
		}
	}
	checkAndClear(resp)
	c.Assert(resp, jc.DeepEquals, &params.ListTemplatesResponse{
		Templates: []params.TemplateResponse{{
			Path: params.EntityPath{"alice", "y"},
			Config: map[string]interface{}{
				"controller": false,
			},
		}, {
			Path: params.EntityPath{"bob", "creds"},
			Config: map[string]interface{}{
				"secret":          "my secret",
				"authorized-keys": sshKey,
				"controller":      false,
			},
		}, {
			Path: params.EntityPath{"bob", "other"},
			Config: map[string]interface{}{
				"controller": true,
			},
		}},
	})

	// Try a similar thing as alice
	resp, err = s.NewClient("alice").ListTemplates(&params.ListTemplates{})
	c.Assert(err, gc.IsNil)

	checkAndClear(resp)
	c.Assert(resp, jc.DeepEquals, &params.ListTemplatesResponse{
		Templates: []params.TemplateResponse{{
			Path: params.EntityPath{"alice", "x"},
			Config: map[string]interface{}{
				"controller":      false,
				"authorized-keys": sshKey,
			},
		}, {
			Path: params.EntityPath{"alice", "y"},
			Config: map[string]interface{}{
				"controller": false,
			},
		}},
	})
}

func (s *APISuite) TestListModelsNoServers(c *gc.C) {
	resp, err := s.NewClient("bob").ListModels(nil)
	c.Assert(err, gc.IsNil)
	c.Assert(resp, jc.DeepEquals, &params.ListModelsResponse{})
}

func (s *APISuite) TestListModelsControllerOnly(c *gc.C) {
	ctlPath := params.EntityPath{"bob", "foo"}
	ctlId := s.assertAddController(c, ctlPath, nil)
	info := s.APIInfo(c)
	resp, err := s.NewClient("bob").ListModels(nil)
	c.Assert(err, gc.IsNil)
	c.Assert(resp, jc.DeepEquals, &params.ListModelsResponse{
		Models: []params.ModelResponse{{
			Path:           ctlId,
			ControllerUUID: info.ModelTag.Id(),
			UUID:           info.ModelTag.Id(),
			CACert:         info.CACert,
			HostPorts:      info.Addrs,
			ControllerPath: ctlPath,
		}},
	})
}

func (s *APISuite) TestListModels(c *gc.C) {
	ctlPath := params.EntityPath{"alice", "foo"}
	ctlId := s.assertAddController(c, ctlPath, nil)
	s.allowModelPerm(c, ctlId)
	s.allowControllerPerm(c, ctlId)
	modelId1, _, uuid1 := s.addModel(c, params.EntityPath{"bob", "bar"}, ctlId)
	modelId2, _, uuid2 := s.addModel(c, params.EntityPath{"charlie", "bar"}, ctlId)
	info := s.APIInfo(c)

	resps := []params.ModelResponse{{
		Path:           ctlId,
		UUID:           info.ModelTag.Id(),
		ControllerUUID: info.ModelTag.Id(),
		CACert:         info.CACert,
		HostPorts:      info.Addrs,
		ControllerPath: ctlPath,
	}, {
		Path:           modelId1,
		UUID:           uuid1,
		ControllerUUID: info.ModelTag.Id(),
		CACert:         info.CACert,
		HostPorts:      info.Addrs,
		ControllerPath: ctlPath,
	}, {
		Path:           modelId2,
		UUID:           uuid2,
		ControllerUUID: info.ModelTag.Id(),
		CACert:         info.CACert,
		HostPorts:      info.Addrs,
		ControllerPath: ctlPath,
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
	ctlId := s.assertAddController(c, params.EntityPath{"alice", "foo"}, nil)

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
	ctlId := s.assertAddController(c, params.EntityPath{"alice", "foo"}, nil)

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

func (s *APISuite) TestAddTemplate(c *gc.C) {
	ctlId := s.assertAddController(c, params.EntityPath{"alice", "foo"}, nil)
	err := s.NewClient("alice").AddTemplate(&params.AddTemplate{
		EntityPath: params.EntityPath{"alice", "creds"},
		Info: params.AddTemplateInfo{
			Controller: ctlId,
			Config: map[string]interface{}{
				"controller":        true,
				"admin-secret":      "my secret",
				"authorized-keys":   sshKey,
				"bootstrap-timeout": 9999,
			},
		},
	})
	c.Assert(err, gc.IsNil)

	// Check that we can get the template and that its secret fields
	// are zeroed out. Note that because we round-trip through
	// JSON, any int fields arrive as float64, but that should be
	// fine in practice.
	tmpl, err := s.NewClient("alice").GetTemplate(&params.GetTemplate{
		EntityPath: params.EntityPath{"alice", "creds"},
	})
	c.Assert(err, gc.IsNil)
	c.Assert(tmpl.Schema, gc.Not(gc.HasLen), 0)
	c.Assert(tmpl.Path, gc.Equals, params.EntityPath{"alice", "creds"})
	c.Assert(tmpl.Config, jc.DeepEquals, map[string]interface{}{
		"controller":        true,
		"admin-secret":      "",
		"authorized-keys":   sshKey,
		"bootstrap-timeout": 9999.0,
	})

	// Check that we can overwrite the template with a new one.
	err = s.NewClient("alice").AddTemplate(&params.AddTemplate{
		EntityPath: params.EntityPath{"alice", "creds"},
		Info: params.AddTemplateInfo{
			Controller: ctlId,
			Config: map[string]interface{}{
				"controller":        false,
				"admin-secret":      "another secret",
				"bootstrap-timeout": 888,
			},
		},
	})
	c.Assert(err, gc.IsNil)

	tmpl, err = s.NewClient("alice").GetTemplate(&params.GetTemplate{
		EntityPath: params.EntityPath{"alice", "creds"},
	})
	c.Assert(err, gc.IsNil)
	c.Assert(tmpl.Schema, gc.Not(gc.HasLen), 0)
	c.Assert(tmpl.Path, gc.Equals, params.EntityPath{"alice", "creds"})
	c.Assert(tmpl.Config, jc.DeepEquals, map[string]interface{}{
		"controller":        false,
		"admin-secret":      "",
		"bootstrap-timeout": 888.0,
	})

	// Check that we can write to another template without affecting
	// the original.
	err = s.NewClient("alice").AddTemplate(&params.AddTemplate{
		EntityPath: params.EntityPath{"alice", "differentcreds"},
		Info: params.AddTemplateInfo{
			Controller: ctlId,
			Config: map[string]interface{}{
				"controller":        true,
				"bootstrap-timeout": 111,
			},
		},
	})
	c.Assert(err, gc.IsNil)

	tmpl, err = s.NewClient("alice").GetTemplate(&params.GetTemplate{
		EntityPath: params.EntityPath{"alice", "differentcreds"},
	})
	c.Assert(err, gc.IsNil)
	c.Assert(tmpl.Path, gc.Equals, params.EntityPath{"alice", "differentcreds"})
	c.Assert(tmpl.Schema, gc.Not(gc.HasLen), 0)
	c.Assert(tmpl.Config, jc.DeepEquals, map[string]interface{}{
		"controller":        true,
		"bootstrap-timeout": 111.0,
	})

	tmpl, err = s.NewClient("alice").GetTemplate(&params.GetTemplate{
		EntityPath: params.EntityPath{"alice", "creds"},
	})
	c.Assert(err, gc.IsNil)
	c.Assert(tmpl.Schema, gc.Not(gc.HasLen), 0)
	c.Assert(tmpl.Config, jc.DeepEquals, map[string]interface{}{
		"controller":        false,
		"admin-secret":      "",
		"bootstrap-timeout": 888.0,
	})
}

func (s *APISuite) TestGetTemplateNotFound(c *gc.C) {
	tmpl, err := s.NewClient("alice").GetTemplate(&params.GetTemplate{
		EntityPath: params.EntityPath{"alice", "xxx"},
	})
	c.Assert(err, gc.ErrorMatches, `GET .*/v2/template/alice/xxx: template "alice/xxx" not found`)
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)
	c.Assert(tmpl, gc.IsNil)
}

var addInvalidTemplateTests = []struct {
	about       string
	config      map[string]interface{}
	ctlId       params.EntityPath
	expectError string
}{{
	about: "unknown key",
	ctlId: params.EntityPath{"alice", "foo"},
	config: map[string]interface{}{
		"badkey": 34565,
	},
	expectError: `PUT .*/v2/template/alice/creds: configuration not compatible with schema: unknown key "badkey" \(value 34565\)`,
}, {
	about: "incompatible type",
	ctlId: params.EntityPath{"alice", "foo"},
	config: map[string]interface{}{
		"admin-secret": 34565,
	},
	expectError: `PUT .*/v2/template/alice/creds: configuration not compatible with schema: admin-secret: expected string, got float64\(34565\)`,
}, {
	about: "unknown controller id",
	ctlId: params.EntityPath{"alice", "bar"},
	config: map[string]interface{}{
		"admin-secret": 34565,
	},
	expectError: `PUT .*/v2/template/alice/creds: cannot get schema for controller: cannot open API: cannot get model: model "alice/bar" not found`,
}}

func (s *APISuite) TestAddInvalidTemplate(c *gc.C) {
	s.assertAddController(c, params.EntityPath{"alice", "foo"}, nil)
	for i, test := range addInvalidTemplateTests {
		c.Logf("test %d: %s", i, test.about)
		err := s.NewClient("alice").AddTemplate(&params.AddTemplate{
			EntityPath: params.EntityPath{"alice", "creds"},
			Info: params.AddTemplateInfo{
				Controller: test.ctlId,
				Config:     test.config,
			},
		})
		c.Assert(err, gc.ErrorMatches, test.expectError)
	}
}

func (s *APISuite) TestDeleteTemplateNotFound(c *gc.C) {
	templPath := params.EntityPath{"alice", "foo"}
	httptesting.AssertJSONCall(c, httptesting.JSONCallParams{
		Method:       "DELETE",
		Handler:      s.JEMSrv,
		URL:          "/v2/template/" + templPath.String(),
		ExpectStatus: http.StatusNotFound,
		ExpectBody: &params.Error{
			Message: `template "alice/foo" not found`,
			Code:    params.ErrNotFound,
		},
		Do: apitest.Do(s.IDMSrv.Client("alice")),
	})
}

func (s *APISuite) TestDeleteTemplate(c *gc.C) {
	ctlId := s.assertAddController(c, params.EntityPath{"alice", "foo"}, nil)
	templPath := params.EntityPath{"alice", "foo"}

	err := s.NewClient("alice").AddTemplate(&params.AddTemplate{
		EntityPath: templPath,
		Info: params.AddTemplateInfo{
			Controller: ctlId,
			Config: map[string]interface{}{
				"controller":        true,
				"admin-secret":      "my secret",
				"authorized-keys":   sshKey,
				"bootstrap-timeout": 9999,
			},
		},
	})
	c.Assert(err, gc.IsNil)

	httptesting.AssertJSONCall(c, httptesting.JSONCallParams{
		Method:       "DELETE",
		Handler:      s.JEMSrv,
		URL:          "/v2/template/" + templPath.String(),
		ExpectStatus: http.StatusOK,
		Do:           apitest.Do(s.IDMSrv.Client("alice")),
	})

	// Check that it is no longer available.
	resp := httptesting.DoRequest(c, httptesting.DoRequestParams{
		Handler: s.JEMSrv,
		URL:     "/v2/template/" + templPath.String(),
		Do:      apitest.Do(s.IDMSrv.Client("alice")),
	})
	c.Assert(resp.Code, gc.Equals, http.StatusNotFound, gc.Commentf("body: %s", resp.Body.Bytes()))
}

func (s *APISuite) TestWhoAmI(c *gc.C) {
	resp, err := s.NewClient("bob").WhoAmI(nil)
	c.Assert(err, gc.IsNil)
	c.Assert(resp.User, gc.Equals, "bob")
}

// addController adds a new controller named name under the
// given user. It returns the controller id.
func (s *APISuite) assertAddController(c *gc.C, ctlPath params.EntityPath, loc map[string]string) params.EntityPath {
	err := s.addController(c, ctlPath, loc)
	c.Assert(err, gc.IsNil)
	return ctlPath
}

func (s *APISuite) addController(c *gc.C, path params.EntityPath, loc map[string]string) error {
	// Note that because the cookies acquired in this request don't
	// persist, the discharge macaroon we get won't affect subsequent
	// requests in the caller.
	info := s.APIInfo(c)

	return s.NewClient(path.User).AddController(&params.AddController{
		EntityPath: path,
		Info: params.ControllerInfo{
			HostPorts:      info.Addrs,
			CACert:         info.CACert,
			User:           info.Tag.Id(),
			Password:       info.Password,
			ControllerUUID: info.ModelTag.Id(),
			Location:       loc,
		},
	})
}

// addModel adds a new model in the given controller. It
// returns the model id.
func (s *APISuite) addModel(c *gc.C, modelPath, ctlPath params.EntityPath) (path params.EntityPath, user, uuid string) {
	// Note that because the cookies acquired in this request don't
	// persist, the discharge macaroon we get won't affect subsequent
	// requests in the caller.

	resp, err := s.NewClient(modelPath.User).NewModel(&params.NewModel{
		User: modelPath.User,
		Info: params.NewModelInfo{
			Name:       modelPath.Name,
			Controller: &ctlPath,
			Config:     dummyModelConfig,
		},
	})
	c.Assert(err, gc.IsNil)
	return resp.Path, resp.User, resp.UUID
}

func (s *APISuite) addTemplate(c *gc.C, tmplPath, ctlPath params.EntityPath, cfg map[string]interface{}) {
	err := s.NewClient(tmplPath.User).AddTemplate(&params.AddTemplate{
		EntityPath: tmplPath,
		Info: params.AddTemplateInfo{
			Controller: ctlPath,
			Config:     cfg,
		},
	})
	c.Assert(err, gc.IsNil)
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

func (s *APISuite) allowTemplatePerm(c *gc.C, path params.EntityPath, acl ...string) {
	if len(acl) == 0 {
		acl = []string{"everyone"}
	}
	err := s.NewClient(path.User).SetTemplatePerm(&params.SetTemplatePerm{
		EntityPath: path,
		ACL: params.ACL{
			Read: acl,
		},
	})
	c.Assert(err, gc.IsNil)
}
