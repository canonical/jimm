package v1_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/usermanager"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/testing/httptesting"
	gc "gopkg.in/check.v1"
	"gopkg.in/errgo.v1"

	"github.com/CanonicalLtd/jem/internal/apitest"
	"github.com/CanonicalLtd/jem/params"
)

type APISuite struct {
	apitest.Suite
}

var _ = gc.Suite(&APISuite{})

const sshKey = "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQDOjaOjVRHchF2RFCKQdgBqrIA5nOoqSprLK47l2th5I675jw+QYMIihXQaITss3hjrh3+5ITyBO41PS5rHLNGtlYUHX78p9CHNZsJqHl/z1Ub1tuMe+/5SY2MkDYzgfPtQtVsLasAIiht/5g78AMMXH3HeCKb9V9cP6/lPPq6mCMvg8TDLrPp/P2vlyukAsJYUvVgoaPDUBpedHbkMj07pDJqe4D7c0yEJ8hQo/6nS+3bh9Q1NvmVNsB1pbtk3RKONIiTAXYcjclmOljxxJnl1O50F5sOIi38vyl7Q63f6a3bXMvJEf1lnPNJKAxspIfEu8gRasny3FEsbHfrxEwVj rog@rog-x220"

var dummyEnvConfig = map[string]interface{}{
	"authorized-keys": sshKey,
	"state-server":    true,
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
	path:   "/v1/model/bob/private",
}, {
	about:  "get controller as non-owner",
	asUser: "other",
	method: "GET",
	path:   "/v1/controller/bob/private",
}, {
	about:  "new model as non-owner",
	asUser: "other",
	method: "POST",
	path:   "/v1/model/bob",
	body: params.NewModelInfo{
		Name:       "newmodel",
		Controller: params.EntityPath{"bob", "open"},
	},
}, {
	about:  "new model with inaccessible controller",
	asUser: "alice",
	method: "POST",
	path:   "/v1/model/alice",
	body: params.NewModelInfo{
		Name:       "newmodel",
		Controller: params.EntityPath{"bob", "private"},
	},
}, {
	about:  "new template as non-owner",
	asUser: "other",
	method: "PUT",
	path:   "/v1/template/bob/something",
	body: params.AddTemplateInfo{
		Controller: params.EntityPath{"bob", "open"},
	},
}, {
	about:  "set controller perm as non-owner",
	asUser: "other",
	method: "PUT",
	path:   "/v1/controller/bob/open/perm",
	body:   params.ACL{},
}, {
	about:  "set model perm as non-owner",
	asUser: "other",
	method: "PUT",
	path:   "/v1/model/bob/open/perm",
	body:   params.ACL{},
}, {
	about:  "set template perm as non-owner",
	asUser: "other",
	method: "PUT",
	path:   "/v1/template/bob/open/perm",
	body:   params.ACL{},
}, {
	about:  "get controller perm as non-owner",
	asUser: "other",
	method: "GET",
	path:   "/v1/controller/bob/private/perm",
}, {
	about:  "get model perm as non-owner",
	asUser: "other",
	method: "GET",
	path:   "/v1/model/bob/private/perm",
}, {
	about:  "get template perm as non-owner",
	asUser: "other",
	method: "GET",
	path:   "/v1/template/bob/private/perm",
}, {
	about:  "get controller perm with ACL that allows us",
	asUser: "other",
	method: "GET",
	path:   "/v1/controller/bob/open/perm",
}, {
	about:  "get model perm with ACL that allows us",
	asUser: "other",
	method: "GET",
	path:   "/v1/model/bob/open/perm",
}, {
	about:  "get template perm with ACL that allows us",
	asUser: "other",
	method: "GET",
	path:   "/v1/template/bob/open/perm",
}}

func (s *APISuite) TestUnauthorized(c *gc.C) {
	s.assertAddController(c, params.EntityPath{"bob", "private"})
	ctlId := s.assertAddController(c, params.EntityPath{"bob", "open"})
	s.addTemplate(c, params.EntityPath{"bob", "open"}, ctlId, dummyEnvConfig)
	s.addTemplate(c, params.EntityPath{"bob", "private"}, ctlId, dummyEnvConfig)

	s.allowServerAllPerm(c, params.EntityPath{"bob", "open"})
	s.allowEnvAllPerm(c, params.EntityPath{"bob", "open"})
	s.allowTemplateAllPerm(c, params.EntityPath{"bob", "private"})

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
			HostPorts: info.Addrs,
			CACert:    info.CACert,
			User:      info.Tag.Id(),
			Password:  info.Password,
			ModelUUID: info.EnvironTag.Id(),
		},
	}, {
		about:    "add model as part of group",
		username: "beatles",
		authUser: "alice",
		body: params.ControllerInfo{
			HostPorts: info.Addrs,
			CACert:    info.CACert,
			User:      info.Tag.Id(),
			Password:  info.Password,
			ModelUUID: info.EnvironTag.Id(),
		},
	}, {
		about:    "incorrect user",
		authUser: "alice",
		username: "bob",
		body: params.ControllerInfo{
			HostPorts: info.Addrs,
			CACert:    info.CACert,
			User:      info.Tag.Id(),
			Password:  info.Password,
			ModelUUID: info.EnvironTag.Id(),
		},
		expectStatus: http.StatusUnauthorized,
		expectBody: params.Error{
			Code:    "unauthorized",
			Message: "unauthorized",
		},
	}, {
		about: "no hosts",
		body: params.ControllerInfo{
			CACert:    info.CACert,
			User:      info.Tag.Id(),
			Password:  info.Password,
			ModelUUID: info.EnvironTag.Id(),
		},
		expectStatus: http.StatusBadRequest,
		expectBody: params.Error{
			Code:    "bad request",
			Message: "no host-ports in request",
		},
	}, {
		about: "no ca-cert",
		body: params.ControllerInfo{
			HostPorts: info.Addrs,
			User:      info.Tag.Id(),
			Password:  info.Password,
			ModelUUID: info.EnvironTag.Id(),
		},
		expectStatus: http.StatusBadRequest,
		expectBody: params.Error{
			Code:    "bad request",
			Message: "no ca-cert in request",
		},
	}, {
		about: "no user",
		body: params.ControllerInfo{
			HostPorts: info.Addrs,
			CACert:    info.CACert,
			Password:  info.Password,
			ModelUUID: info.EnvironTag.Id(),
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
			HostPorts: []string{"0.1.2.3:1234"},
			CACert:    info.CACert,
			User:      info.Tag.Id(),
			Password:  info.Password,
			ModelUUID: info.EnvironTag.Id(),
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
			URL:          fmt.Sprintf("/v1/controller/%s", modelPath),
			Do:           apitest.Do(client),
			ExpectStatus: test.expectStatus,
			ExpectBody:   test.expectBody,
		})
		if test.expectStatus != 0 {
			continue
		}
		// The server was added successfully. Check that we
		// can fetch its associated model and that we
		// can connect to that.
		modelResp, err := s.NewClient(authUser).GetModel(&params.GetModel{
			EntityPath: modelPath,
		})
		c.Assert(err, gc.IsNil)
		c.Assert(modelResp, jc.DeepEquals, &params.ModelResponse{
			Path:      modelPath,
			User:      test.body.User,
			Password:  test.body.Password,
			HostPorts: test.body.HostPorts,
			CACert:    test.body.CACert,
			UUID:      test.body.ModelUUID,
		})
		st := openAPIFromModelResponse(c, modelResp)
		st.Close()
		// Clear the connection pool for the next test.
		s.JEMSrv.Pool().ClearAPIConnCache()
	}
}

func (s *APISuite) TestAddControllerDuplicate(c *gc.C) {
	ctlPath := s.assertAddController(c, params.EntityPath{"bob", "dupmodel"})
	err := s.addController(c, ctlPath)
	c.Assert(err, gc.ErrorMatches, "PUT http://.*: already exists")
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrAlreadyExists)
}

func (s *APISuite) TestAddControllerUnauthenticated(c *gc.C) {
	httptesting.AssertJSONCall(c, httptesting.JSONCallParams{
		Method:  "PUT",
		Handler: s.JEMSrv,
		URL:     "/v1/controller/user/model",
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
		Header:  map[string][]string{"Bakery-Protocol-Version": []string{"1"}},
		URL:     "/v1/controller/user/model",
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
		URL:     "/v1/model/user/foo",
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
		URL:     "/v1/model/user/foo",
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
		URL:     "/v1/model/user/foo",
		ExpectBody: &params.Error{
			Message: `model "user/foo" not found`,
			Code:    params.ErrNotFound,
		},
		ExpectStatus: http.StatusNotFound,
		Do:           apitest.Do(s.IDMSrv.Client("user")),
	})
}

func (s *APISuite) TestDeleteModel(c *gc.C) {
	ctlId := s.assertAddController(c, params.EntityPath{"bob", "who"})
	modelPath := params.EntityPath{"bob", "foobarred"}
	modelId, user, uuid := s.addModel(c, modelPath, ctlId)
	resp := httptesting.DoRequest(c, httptesting.DoRequestParams{
		Handler: s.JEMSrv,
		URL:     "/v1/model/" + modelId.String(),
		Do:      apitest.Do(s.IDMSrv.Client("bob")),
	})
	c.Assert(resp.Code, gc.Equals, http.StatusOK, gc.Commentf("body: %s", resp.Body.Bytes()))
	c.Assert(user, gc.NotNil)
	c.Assert(uuid, gc.NotNil)

	// Delete model.
	httptesting.AssertJSONCall(c, httptesting.JSONCallParams{
		Method:       "DELETE",
		Handler:      s.JEMSrv,
		URL:          "/v1/model/" + modelId.String(),
		ExpectStatus: http.StatusOK,
		Do:           apitest.Do(s.IDMSrv.Client("bob")),
	})
	// Check that it doesn't exist anymore.
	httptesting.AssertJSONCall(c, httptesting.JSONCallParams{
		Handler:      s.JEMSrv,
		URL:          "/v1/model/" + modelId.String(),
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
		URL:          "/v1/model/" + ctlId.String(),
		ExpectStatus: http.StatusForbidden,
		ExpectBody: &params.Error{
			Message: `cannot remove model "bob/who" because it is a controller`,
			Code:    params.ErrForbidden,
		},
		Do: apitest.Do(s.IDMSrv.Client("bob")),
	})
}

func (s *APISuite) TestGetController(c *gc.C) {
	ctlId := s.assertAddController(c, params.EntityPath{"bob", "foo"})

	resp := httptesting.DoRequest(c, httptesting.DoRequestParams{
		Handler: s.JEMSrv,
		URL:     "/v1/controller/" + ctlId.String(),
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

func (s *APISuite) TestGetControllerNotFound(c *gc.C) {
	httptesting.AssertJSONCall(c, httptesting.JSONCallParams{
		Method:  "GET",
		Handler: s.JEMSrv,
		URL:     "/v1/controller/bob/foo",
		ExpectBody: &params.Error{
			Message: `cannot open API: cannot get model: model "bob/foo" not found`,
			Code:    params.ErrNotFound,
		},
		ExpectStatus: http.StatusNotFound,
		Do:           apitest.Do(s.IDMSrv.Client("bob")),
	})

	// Any other user just sees Unauthorized.
	httptesting.AssertJSONCall(c, httptesting.JSONCallParams{
		Method:  "GET",
		Handler: s.JEMSrv,
		URL:     "/v1/controller/bob/foo",
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
		URL:     "/v1/controller/bob/foobarred",
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
	ctlId := s.assertAddController(c, params.EntityPath{"bob", "foobarred"})
	// Add controller to JEM.
	resp := httptesting.DoRequest(c, httptesting.DoRequestParams{
		Handler: s.JEMSrv,
		URL:     "/v1/controller/" + ctlId.String(),
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
		URL:          "/v1/controller/bob/foobarred",
		ExpectStatus: http.StatusOK,
		Do:           apitest.Do(s.IDMSrv.Client("bob")),
	})
	// Check that it doesn't exist anymore.
	httptesting.AssertJSONCall(c, httptesting.JSONCallParams{
		Handler:      s.JEMSrv,
		URL:          "/v1/controller/" + ctlId.String(),
		Do:           apitest.Do(s.IDMSrv.Client("bob")),
		ExpectStatus: http.StatusNotFound,
		ExpectBody: params.Error{
			Message: `cannot open API: cannot get model: model "bob/foobarred" not found`,
			Code:    params.ErrNotFound,
		},
	})
	// Check that its models doesn't exist.
	httptesting.AssertJSONCall(c, httptesting.JSONCallParams{
		Handler:      s.JEMSrv,
		URL:          "/v1/model/" + ctlId.String(),
		Do:           apitest.Do(s.IDMSrv.Client("bob")),
		ExpectStatus: http.StatusNotFound,
		ExpectBody: params.Error{
			Message: `model "bob/foobarred" not found`,
			Code:    params.ErrNotFound,
		},
	})
	httptesting.AssertJSONCall(c, httptesting.JSONCallParams{
		Handler:      s.JEMSrv,
		URL:          "/v1/model/" + modelId.String(),
		Do:           apitest.Do(s.IDMSrv.Client("bob")),
		ExpectStatus: http.StatusNotFound,
		ExpectBody: params.Error{
			Message: `model "bob/bar" not found`,
			Code:    params.ErrNotFound,
		},
	})
}

func (s *APISuite) TestNewModel(c *gc.C) {
	ctlId := s.assertAddController(c, params.EntityPath{"bob", "foo"})

	var modelRespBody json.RawMessage
	httptesting.AssertJSONCall(c, httptesting.JSONCallParams{
		Method:  "POST",
		URL:     "/v1/model/bob",
		Handler: s.JEMSrv,
		JSONBody: params.NewModelInfo{
			Name:       params.Name("bar"),
			Controller: ctlId,
			Config:     dummyEnvConfig,
			Password:   "secret",
		},
		ExpectBody: httptesting.BodyAsserter(func(_ *gc.C, body json.RawMessage) {
			modelRespBody = body
		}),
		Do: apitest.Do(s.IDMSrv.Client("bob")),
	})
	var modelResp params.ModelResponse
	err := json.Unmarshal(modelRespBody, &modelResp)
	c.Assert(err, gc.IsNil)

	c.Assert(modelResp.ControllerUUID, gc.Equals, s.APIInfo(c).EnvironTag.Id())

	st := openAPIFromModelResponse(c, &modelResp)
	st.Close()

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
	st.Close()
}

func (s *APISuite) TestNewModelWithTemplates(c *gc.C) {
	ctlId := s.assertAddController(c, params.EntityPath{"bob", "foo"})

	// TODO change "admin-secret" to "secret" when we can
	// make the "secret" configuration attribute marked as secret
	// in the schema.
	s.addTemplate(c, params.EntityPath{"bob", "creds"}, ctlId, map[string]interface{}{
		"secret":          "my secret",
		"authorized-keys": sshKey,
		"state-server":    false,
	})
	s.addTemplate(c, params.EntityPath{"bob", "other"}, ctlId, map[string]interface{}{
		"state-server": true,
	})

	modelPath := params.EntityPath{"bob", "model"}
	resp, err := s.NewClient("bob").NewModel(&params.NewModel{
		User: modelPath.User,
		Info: params.NewModelInfo{
			Name:       modelPath.Name,
			Controller: ctlId,
			Config: map[string]interface{}{
				"secret": "another secret",
			},
			Password:      "user secret",
			TemplatePaths: []params.EntityPath{{"bob", "creds"}, {"bob", "other"}},
		},
	})
	c.Assert(err, gc.IsNil)

	// Check that the model was actually created with the right
	// configuration.
	m, err := s.State.GetEnvironment(names.NewEnvironTag(resp.UUID))
	c.Assert(err, gc.IsNil)
	cfg, err := m.Config()
	c.Assert(err, gc.IsNil)
	attrs := cfg.AllAttrs()
	c.Assert(attrs["state-server"], gc.Equals, true)
	c.Assert(attrs["secret"], gc.Equals, "another secret")
	c.Assert(attrs["authorized-keys"], gc.Equals, sshKey)

	st := openAPIFromModelResponse(c, resp)
	st.Close()
}

func (s *APISuite) TestNewModelWithTemplateNotFound(c *gc.C) {
	ctlId := s.assertAddController(c, params.EntityPath{"bob", "foo"})

	resp, err := s.NewClient("bob").NewModel(&params.NewModel{
		User: "bob",
		Info: params.NewModelInfo{
			Name:       "x",
			Controller: ctlId,
			Config: map[string]interface{}{
				"secret": "another secret",
			},
			Password:      "user secret",
			TemplatePaths: []params.EntityPath{{"bob", "creds"}},
		},
	})
	c.Assert(err, gc.ErrorMatches, `POST .*/v1/model/bob: cannot get template "bob/creds": template "bob/creds" not found`)
	c.Assert(resp, gc.IsNil)
}

func openAPIFromModelResponse(c *gc.C, resp *params.ModelResponse) api.Connection {
	// Ensure that we can connect to the new model
	apiInfo := &api.Info{
		Tag:        names.NewUserTag(resp.User),
		Password:   resp.Password,
		Addrs:      resp.HostPorts,
		CACert:     resp.CACert,
		EnvironTag: names.NewEnvironTag(resp.UUID),
	}
	st, err := api.Open(apiInfo, api.DialOpts{})
	c.Assert(err, gc.IsNil, gc.Commentf("user: %q; password: %q", resp.User, resp.Password))
	return st
}

func (s *APISuite) TestNewModelUnderGroup(c *gc.C) {
	ctlId := s.assertAddController(c, params.EntityPath{"bob", "foo"})

	s.IDMSrv.AddUser("bob", "beatles")
	var modelRespBody json.RawMessage
	httptesting.AssertJSONCall(c, httptesting.JSONCallParams{
		Method:  "POST",
		URL:     "/v1/model/beatles",
		Handler: s.JEMSrv,
		JSONBody: params.NewModelInfo{
			Name:       params.Name("bar"),
			Controller: ctlId,
			Config:     dummyEnvConfig,
			Password:   "secret",
		},
		ExpectBody: httptesting.BodyAsserter(func(_ *gc.C, body json.RawMessage) {
			modelRespBody = body
		}),
		Do: apitest.Do(s.IDMSrv.Client("bob")),
	})
	var modelResp params.ModelResponse
	err := json.Unmarshal(modelRespBody, &modelResp)
	c.Assert(err, gc.IsNil)

	c.Assert(modelResp.ControllerUUID, gc.Equals, s.APIInfo(c).EnvironTag.Id())

	// Ensure that we can connect to the new model
	apiInfo := &api.Info{
		Tag:        names.NewUserTag(string(modelResp.User)),
		Password:   "secret",
		Addrs:      modelResp.HostPorts,
		CACert:     modelResp.CACert,
		EnvironTag: names.NewEnvironTag(modelResp.UUID),
	}
	st, err := api.Open(apiInfo, api.DialOpts{})
	c.Assert(err, gc.IsNil)
	defer st.Close()
}

func (s *APISuite) TestNewModelWithExistingUser(c *gc.C) {
	username := "jem-bob--bar"

	_, err := usermanager.NewClient(s.APIState).AddUser(username, "", "old")
	c.Assert(err, gc.IsNil)

	ctlId := s.assertAddController(c, params.EntityPath{"bob", "foo"})

	var modelRespBody json.RawMessage
	httptesting.AssertJSONCall(c, httptesting.JSONCallParams{
		Method:  "POST",
		URL:     "/v1/model/bob",
		Handler: s.JEMSrv,
		JSONBody: params.NewModelInfo{
			Name:       params.Name("bar"),
			Controller: ctlId,
			Config:     dummyEnvConfig,
			Password:   "secret",
		},
		ExpectBody: httptesting.BodyAsserter(func(_ *gc.C, body json.RawMessage) {
			modelRespBody = body
		}),
		Do: apitest.Do(s.IDMSrv.Client("bob")),
	})
	var modelResp params.ModelResponse
	err = json.Unmarshal(modelRespBody, &modelResp)
	c.Assert(err, gc.IsNil)

	c.Assert(modelResp.ControllerUUID, gc.Equals, s.APIInfo(c).EnvironTag.Id())

	// Make sure that we really are reusing the username.
	c.Assert(modelResp.User, gc.Equals, username)

	// Ensure that we can connect to the new model with
	// the new secret
	apiInfo := &api.Info{
		Tag:        names.NewUserTag(username),
		Password:   modelResp.Password,
		Addrs:      modelResp.HostPorts,
		CACert:     modelResp.CACert,
		EnvironTag: names.NewEnvironTag(modelResp.UUID),
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
			URL:     "/v1/model/bob",
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
		URL:     "/v1/model/bob",
		Handler: s.JEMSrv,
		JSONBody: params.NewModelInfo{
			Name:       params.Name("bar"),
			Controller: params.EntityPath{"bob", "foo"},
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
	ctlId := s.assertAddController(c, params.EntityPath{"bob", "foo"})

	httptesting.AssertJSONCall(c, httptesting.JSONCallParams{
		Method:  "POST",
		URL:     "/v1/model/bob",
		Handler: s.JEMSrv,
		JSONBody: params.NewModelInfo{
			Name:       params.Name("bar"),
			Controller: ctlId,
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
	ctlId := s.assertAddController(c, params.EntityPath{"bob", "foo"})

	body := &params.NewModelInfo{
		Name:       "bar",
		Password:   "password",
		Controller: ctlId,
		Config:     dummyEnvConfig,
	}
	p := httptesting.JSONCallParams{
		Method:     "POST",
		URL:        "/v1/model/bob",
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

func (s *APISuite) TestNewModelWithNoPassword(c *gc.C) {
	ctlId := s.assertAddController(c, params.EntityPath{"bob", "foo"})

	// N.B. "state-server" is a required attribute
	httptesting.AssertJSONCall(c, httptesting.JSONCallParams{
		Method:  "POST",
		URL:     "/v1/model/bob",
		Handler: s.JEMSrv,
		JSONBody: params.NewModelInfo{
			Name:       "bar",
			Controller: ctlId,
			Config: map[string]interface{}{
				"authorized-keys": sshKey,
			},
		},
		ExpectBody: params.Error{
			Code:    params.ErrBadRequest,
			Message: `cannot create user: no password specified`,
		},
		ExpectStatus: http.StatusBadRequest,
		Do:           apitest.Do(s.IDMSrv.Client("bob")),
	})
}

func (s *APISuite) TestNewModelCannotCreate(c *gc.C) {
	ctlId := s.assertAddController(c, params.EntityPath{"bob", "foo"})

	// N.B. "state-server" is a required attribute
	httptesting.AssertJSONCall(c, httptesting.JSONCallParams{
		Method:  "POST",
		URL:     "/v1/model/bob",
		Handler: s.JEMSrv,
		JSONBody: params.NewModelInfo{
			Name:       "bar",
			Password:   "secret",
			Controller: ctlId,
			Config: map[string]interface{}{
				"authorized-keys": sshKey,
			},
		},
		ExpectBody: params.Error{
			Message: `cannot create model: provider validation failed: state-server: expected bool, got nothing`,
		},
		ExpectStatus: http.StatusInternalServerError,
		Do:           apitest.Do(s.IDMSrv.Client("bob")),
	})

	// Check that the model is not there (it was added temporarily during the call).
	httptesting.AssertJSONCall(c, httptesting.JSONCallParams{
		Method:  "GET",
		Handler: s.JEMSrv,
		URL:     "/v1/model/bob/bar",
		ExpectBody: &params.Error{
			Message: `model "bob/bar" not found`,
			Code:    params.ErrNotFound,
		},
		ExpectStatus: http.StatusNotFound,
		Do:           apitest.Do(s.IDMSrv.Client("bob")),
	})
}

func (s *APISuite) TestNewModelUnauthorized(c *gc.C) {
	ctlId := s.assertAddController(c, params.EntityPath{"bob", "foo"})

	httptesting.AssertJSONCall(c, httptesting.JSONCallParams{
		Method:  "POST",
		URL:     "/v1/model/bob",
		Handler: s.JEMSrv,
		JSONBody: params.NewModelInfo{
			Name:       "bar",
			Controller: ctlId,
			Config:     dummyEnvConfig,
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
	ctlId := s.assertAddController(c, params.EntityPath{"bob", "foo"})
	resp, err := s.NewClient("bob").ListController(nil)
	c.Assert(err, gc.IsNil)
	c.Assert(resp, jc.DeepEquals, &params.ListControllerResponse{
		Controllers: []params.ControllerResponse{{
			Path: ctlId,
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
	ctlId := s.assertAddController(c, params.EntityPath{"bob", "foo"})
	s.addTemplate(c, params.EntityPath{"bob", "other"}, ctlId, map[string]interface{}{
		"state-server": true,
	})
	s.addTemplate(c, params.EntityPath{"bob", "creds"}, ctlId, map[string]interface{}{
		"secret":          "my secret",
		"authorized-keys": sshKey,
		"state-server":    false,
	})
	s.addTemplate(c, params.EntityPath{"alice", "x"}, ctlId, map[string]interface{}{
		"state-server":    false,
		"authorized-keys": sshKey,
	})
	s.addTemplate(c, params.EntityPath{"alice", "y"}, ctlId, map[string]interface{}{
		"state-server": false,
	})
	s.allowTemplateAllPerm(c, params.EntityPath{"alice", "y"})

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
				"state-server": false,
			},
		}, {
			Path: params.EntityPath{"bob", "creds"},
			Config: map[string]interface{}{
				"secret":          "my secret",
				"authorized-keys": sshKey,
				"state-server":    false,
			},
		}, {
			Path: params.EntityPath{"bob", "other"},
			Config: map[string]interface{}{
				"state-server": true,
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
				"state-server":    false,
				"authorized-keys": sshKey,
			},
		}, {
			Path: params.EntityPath{"alice", "y"},
			Config: map[string]interface{}{
				"state-server": false,
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
	ctlId := s.assertAddController(c, params.EntityPath{"bob", "foo"})
	info := s.APIInfo(c)
	resp, err := s.NewClient("bob").ListModels(nil)
	c.Assert(err, gc.IsNil)
	c.Assert(resp, jc.DeepEquals, &params.ListModelsResponse{
		Models: []params.ModelResponse{{
			Path:      ctlId,
			User:      info.Tag.Id(),
			Password:  info.Password,
			UUID:      info.EnvironTag.Id(),
			CACert:    info.CACert,
			HostPorts: info.Addrs,
		}},
	})
}

func (s *APISuite) allowServerAllPerm(c *gc.C, path params.EntityPath) {
	err := s.NewClient(path.User).SetControllerPerm(&params.SetControllerPerm{
		EntityPath: path,
		ACL: params.ACL{
			Read: []string{"everyone"},
		},
	})
	c.Assert(err, gc.IsNil)
}

func (s *APISuite) allowEnvAllPerm(c *gc.C, path params.EntityPath) {
	err := s.NewClient(path.User).SetModelPerm(&params.SetModelPerm{
		EntityPath: path,
		ACL: params.ACL{
			Read: []string{"everyone"},
		},
	})
	c.Assert(err, gc.IsNil)
}

func (s *APISuite) allowTemplateAllPerm(c *gc.C, path params.EntityPath) {
	err := s.NewClient(path.User).SetTemplatePerm(&params.SetTemplatePerm{
		EntityPath: path,
		ACL: params.ACL{
			Read: []string{"everyone"},
		},
	})
	c.Assert(err, gc.IsNil)
}

func (s *APISuite) TestListModels(c *gc.C) {
	ctlId := s.assertAddController(c, params.EntityPath{"alice", "foo"})
	s.allowEnvAllPerm(c, ctlId)
	s.allowServerAllPerm(c, ctlId)
	modelId1, user1, uuid1 := s.addModel(c, params.EntityPath{"bob", "bar"}, ctlId)
	modelId2, user2, uuid2 := s.addModel(c, params.EntityPath{"charlie", "bar"}, ctlId)
	info := s.APIInfo(c)

	resps := []params.ModelResponse{{
		Path:      ctlId,
		User:      info.Tag.Id(),
		Password:  info.Password,
		UUID:      info.EnvironTag.Id(),
		CACert:    info.CACert,
		HostPorts: info.Addrs,
	}, {
		Path:      modelId1,
		User:      user1,
		Password:  info.Password,
		UUID:      uuid1,
		CACert:    info.CACert,
		HostPorts: info.Addrs,
	}, {
		Path:      modelId2,
		User:      user2,
		Password:  info.Password,
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
	ctlId := s.assertAddController(c, params.EntityPath{"alice", "foo"})

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
	ctlId := s.assertAddController(c, params.EntityPath{"alice", "foo"})

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
	ctlId := s.assertAddController(c, params.EntityPath{"alice", "foo"})
	err := s.NewClient("alice").AddTemplate(&params.AddTemplate{
		EntityPath: params.EntityPath{"alice", "creds"},
		Info: params.AddTemplateInfo{
			Controller: ctlId,
			Config: map[string]interface{}{
				"state-server":      true,
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
		"state-server":      true,
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
				"state-server":      false,
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
		"state-server":      false,
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
				"state-server":      true,
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
		"state-server":      true,
		"bootstrap-timeout": 111.0,
	})

	tmpl, err = s.NewClient("alice").GetTemplate(&params.GetTemplate{
		EntityPath: params.EntityPath{"alice", "creds"},
	})
	c.Assert(err, gc.IsNil)
	c.Assert(tmpl.Schema, gc.Not(gc.HasLen), 0)
	c.Assert(tmpl.Config, jc.DeepEquals, map[string]interface{}{
		"state-server":      false,
		"admin-secret":      "",
		"bootstrap-timeout": 888.0,
	})
}

func (s *APISuite) TestGetTemplateNotFound(c *gc.C) {
	tmpl, err := s.NewClient("alice").GetTemplate(&params.GetTemplate{
		EntityPath: params.EntityPath{"alice", "xxx"},
	})
	c.Assert(err, gc.ErrorMatches, `GET .*/v1/template/alice/xxx: template "alice/xxx" not found`)
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
	expectError: `PUT .*/v1/template/alice/creds: configuration not compatible with schema: unknown key "badkey" \(value 34565\)`,
}, {
	about: "incompatible type",
	ctlId: params.EntityPath{"alice", "foo"},
	config: map[string]interface{}{
		"admin-secret": 34565,
	},
	expectError: `PUT .*/v1/template/alice/creds: configuration not compatible with schema: admin-secret: expected string, got float64\(34565\)`,
}, {
	about: "unknown controller id",
	ctlId: params.EntityPath{"alice", "bar"},
	config: map[string]interface{}{
		"admin-secret": 34565,
	},
	expectError: `PUT .*/v1/template/alice/creds: cannot get schema for controller: cannot open API: cannot get model: model "alice/bar" not found`,
}}

func (s *APISuite) TestAddInvalidTemplate(c *gc.C) {
	s.assertAddController(c, params.EntityPath{"alice", "foo"})
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
		URL:          "/v1/template/" + templPath.String(),
		ExpectStatus: http.StatusNotFound,
		ExpectBody: &params.Error{
			Message: `template "alice/foo" not found`,
			Code:    params.ErrNotFound,
		},
		Do: apitest.Do(s.IDMSrv.Client("alice")),
	})
}

func (s *APISuite) TestDeleteTemplate(c *gc.C) {
	ctlId := s.assertAddController(c, params.EntityPath{"alice", "foo"})
	templPath := params.EntityPath{"alice", "foo"}

	err := s.NewClient("alice").AddTemplate(&params.AddTemplate{
		EntityPath: templPath,
		Info: params.AddTemplateInfo{
			Controller: ctlId,
			Config: map[string]interface{}{
				"state-server":      true,
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
		URL:          "/v1/template/" + templPath.String(),
		ExpectStatus: http.StatusOK,
		Do:           apitest.Do(s.IDMSrv.Client("alice")),
	})

	// Check that it is no longer available.
	resp := httptesting.DoRequest(c, httptesting.DoRequestParams{
		Handler: s.JEMSrv,
		URL:     "/v1/template/" + templPath.String(),
		Do:      apitest.Do(s.IDMSrv.Client("alice")),
	})
	c.Assert(resp.Code, gc.Equals, http.StatusNotFound, gc.Commentf("body: %s", resp.Body.Bytes()))
}

func (s *APISuite) TestWhoAmI(c *gc.C) {
	resp, err := s.NewClient("bob").WhoAmI(nil)
	c.Assert(err, gc.IsNil)
	c.Assert(resp.User, gc.Equals, "bob")
}

// addController adds a new stateserver named name under the
// given user. It returns the controller id.
func (s *APISuite) assertAddController(c *gc.C, ctlPath params.EntityPath) params.EntityPath {
	err := s.addController(c, ctlPath)
	c.Assert(err, gc.IsNil)
	return ctlPath
}

func (s *APISuite) addController(c *gc.C, path params.EntityPath) error {
	// Note that because the cookies acquired in this request don't
	// persist, the discharge macaroon we get won't affect subsequent
	// requests in the caller.
	info := s.APIInfo(c)

	return s.NewClient(path.User).AddController(&params.AddController{
		EntityPath: path,
		Info: params.ControllerInfo{
			HostPorts: info.Addrs,
			CACert:    info.CACert,
			User:      info.Tag.Id(),
			Password:  info.Password,
			ModelUUID: info.EnvironTag.Id(),
		},
	})
}

// addModel adds a new model in the given controller. It
// returns the model id.
func (s *APISuite) addModel(c *gc.C, modelPath, ctlPath params.EntityPath) (path params.EntityPath, user, uuid string) {
	// Note that because the cookies acquired in this request don't
	// persist, the discharge macaroon we get won't affect subsequent
	// requests in the caller.

	info := s.APIInfo(c)
	resp, err := s.NewClient(modelPath.User).NewModel(&params.NewModel{
		User: modelPath.User,
		Info: params.NewModelInfo{
			Name:       modelPath.Name,
			Password:   info.Password,
			Controller: ctlPath,
			Config:     dummyEnvConfig,
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
