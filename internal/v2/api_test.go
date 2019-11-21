package v2_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/juju/juju/api"
	controllerapi "github.com/juju/juju/api/controller"
	jujuparams "github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/component/all"
	"github.com/juju/juju/network"
	jujuversion "github.com/juju/juju/version"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/testing/httptesting"
	"github.com/juju/utils"
	"go.uber.org/zap/zapcore"
	gc "gopkg.in/check.v1"
	"gopkg.in/errgo.v1"
	"gopkg.in/juju/names.v2"

	"github.com/CanonicalLtd/jimm/internal/apitest"
	"github.com/CanonicalLtd/jimm/internal/jemtest"
	"github.com/CanonicalLtd/jimm/internal/mongodoc"
	v2 "github.com/CanonicalLtd/jimm/internal/v2"
	"github.com/CanonicalLtd/jimm/internal/zapctx"
	"github.com/CanonicalLtd/jimm/params"
)

func init() {
	all.RegisterForServer()
}

type APISuite struct {
	apitest.Suite
}

var _ = gc.Suite(&APISuite{})

var testContext = context.Background()

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
		Credential: params.CredentialPath{
			Cloud: "dummy",
			User:  "bob",
			Name:  "cred1",
		},
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
}, {
	about:  "set deprecated status as non-owner",
	asUser: "other",
	method: "PUT",
	path:   "/v2/controller/bob/open",
	body:   params.DeprecatedBody{},
}}

func (s *APISuite) TestUnauthorized(c *gc.C) {
	s.AssertAddController(c, params.EntityPath{"bob", "private"}, false)
	s.AssertAddController(c, params.EntityPath{"bob", "open"}, false)

	cred := s.AssertUpdateCredential(c, "bob", "dummy", "cred1", "empty")
	s.AssertUpdateCredential(c, "alice", "dummy", "cred1", "empty")
	s.CreateModel(c, params.EntityPath{"bob", "open"}, params.EntityPath{"bob", "open"}, cred)

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
			Public:         true,
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
		},
		expectStatus: http.StatusBadRequest,
		expectBody: httptesting.BodyAsserter(func(c *gc.C, m json.RawMessage) {
			var body params.Error
			err := json.Unmarshal(m, &body)
			c.Assert(err, gc.Equals, nil)
			c.Assert(body.Code, gc.Equals, params.ErrBadRequest)
			c.Assert(body.Message, gc.Matches, `cannot connect to controller: unable to connect to API: .*`)
		}),
	}, {
		about: "controller with additional host address",
		body: params.ControllerInfo{
			HostPorts:      append(info.Addrs, "example.com:443"),
			CACert:         info.CACert,
			User:           info.Tag.Id(),
			Password:       info.Password,
			ControllerUUID: info.ModelTag.Id(),
			Public:         true,
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
		conn, err := s.JEM.OpenAPI(context.TODO(), controllerPath)
		c.Assert(err, gc.Equals, nil)
		conn.Close()

		// Check that the version has been set correctly.
		ctl, err := s.JEM.DB.Controller(context.TODO(), controllerPath)
		c.Assert(err, gc.Equals, nil)
		v, ok := conn.ServerVersion()
		c.Assert(ok, gc.Equals, true)
		c.Assert(ctl.Version, jc.DeepEquals, &v)

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
	err := s.JEM.DB.SetControllerUnavailableAt(testContext, ctlId, t)
	c.Assert(err, gc.Equals, nil)

	resp := httptesting.DoRequest(c, httptesting.DoRequestParams{
		Handler: s.JEMSrv,
		URL:     "/v2/controller/" + ctlId.String(),
		Do:      apitest.Do(s.IDMSrv.Client("bob")),
	})
	c.Assert(resp.Code, gc.Equals, http.StatusOK, gc.Commentf("body: %s", resp.Body.Bytes()))
	var controllerInfo params.ControllerResponse
	err = json.Unmarshal(resp.Body.Bytes(), &controllerInfo)
	c.Assert(err, gc.IsNil, gc.Commentf("body: %s", resp.Body.String()))
	c.Assert((*controllerInfo.UnavailableSince).UTC(), jc.DeepEquals, mongodoc.Time(t).UTC())
	c.Assert(controllerInfo.Location, jc.DeepEquals, map[string]string{
		"cloud":  "dummy",
		"region": "dummy-region",
	})
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
		},
	})
	c.Assert(err, gc.Equals, nil)

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
			Credential: params.CredentialPath{
				Cloud: "dummy",
				User:  "bob",
				Name:  cred,
			},
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
	c.Assert(err, gc.Equals, nil)

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
	c.Assert(err, gc.Equals, nil)
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
		Name: "test-model",
		Credential: params.CredentialPath{
			Cloud: "dummy",
			User:  "alice",
			Name:  "cred1",
		},
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
		Name: "test-model",
		Credential: params.CredentialPath{
			Cloud: "dummy",
			User:  "alice",
			Name:  "cred1",
		},
		Location: map[string]string{
			"cloud": "aws",
		},
		Config: map[string]interface{}{
			"secret": "a secret",
		},
	},
	expectError:      `cloud "aws" region "" not found`,
	expectErrorCause: params.ErrNotFound,
}, {
	about: "no matching region",
	user:  "alice",
	info: params.NewModelInfo{
		Name: "test-model",
		Credential: params.CredentialPath{
			Cloud: "dummy",
			User:  "alice",
			Name:  "cred1",
		},
		Location: map[string]string{
			"region": "us-east-1",
		},
		Config: map[string]interface{}{
			"secret": "a secret",
		},
	},
	expectError:      `cloud "" region "us-east-1" not found`,
	expectErrorCause: params.ErrNotFound,
}, {
	about: "unrecognised location parameter",
	user:  "alice",
	info: params.NewModelInfo{
		Name: "test-model",
		Credential: params.CredentialPath{
			Cloud: "dummy",
			User:  "alice",
			Name:  "cred1",
		},
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
		Name: "test-model",
		Credential: params.CredentialPath{
			Cloud: "aws",
			User:  "alice",
			Name:  "cred1",
		},
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
		Name: "test-model",
		Credential: params.CredentialPath{
			Cloud: "aws",
			User:  "alice",
			Name:  "cred1",
		},
		Location: map[string]string{
			"cloud": "bad/name",
		},
		Config: map[string]interface{}{
			"secret": "a secret",
		},
	},
	expectError:      `invalid cloud "bad/name"`,
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
		c.Assert(err, gc.Equals, nil)
		c.Assert(resp.Path, jc.DeepEquals, params.EntityPath{test.user, test.info.Name})
		c.Assert(resp.ControllerPath, jc.DeepEquals, ctlId)
	}
}

func (s *APISuite) assertModelConfigAttr(c *gc.C, modelPath params.EntityPath, attr string, val interface{}) {
	m, err := s.JEM.DB.Model(testContext, modelPath)
	c.Assert(err, gc.Equals, nil)
	st, err := s.StatePool.Get(m.UUID)
	c.Assert(err, gc.Equals, nil)
	defer st.Release()
	stm, err := st.Model()
	c.Assert(err, gc.Equals, nil)
	cfg, err := stm.Config()
	c.Assert(err, gc.Equals, nil)
	c.Assert(cfg.AllAttrs()[attr], jc.DeepEquals, val)
}

func (s *APISuite) TestGetModelWhenControllerUnavailable(c *gc.C) {
	info := s.APIInfo(c)
	ctlId := s.AssertAddController(c, params.EntityPath{"bob", "foo"}, false)
	aCred := s.AssertUpdateCredential(c, "bob", "dummy", "cred1", "empty")
	aModel, aUUID := s.CreateModel(c, params.EntityPath{"bob", "foo"}, ctlId, aCred)

	err := s.JEM.DB.SetModelLife(testContext, aModel, aUUID, "dying")
	c.Assert(err, gc.Equals, nil)
	t := time.Now()
	err = s.JEM.DB.SetControllerUnavailableAt(testContext, ctlId, t)
	c.Assert(err, gc.Equals, nil)

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
	c.Assert(err, gc.Equals, nil)
	c.Assert(modelResp, jc.DeepEquals, params.ModelResponse{
		Path:             aModel,
		UUID:             aUUID,
		ControllerPath:   ctlId,
		ControllerUUID:   s.ControllerConfig.ControllerUUID(),
		CACert:           info.CACert,
		HostPorts:        info.Addrs,
		Life:             "dying",
		UnavailableSince: newTime(mongodoc.Time(t).UTC()),
		Creator:          "bob",
	})
}

func (s *APISuite) TestGetModelWithCounts(c *gc.C) {
	ctlId := s.AssertAddController(c, params.EntityPath{"bob", "foo"}, false)
	cred := s.AssertUpdateCredential(c, "bob", "dummy", "cred1", "empty")
	modelId, uuid := s.CreateModel(c, params.EntityPath{"bob", "foo"}, ctlId, cred)

	t0 := time.Unix(0, 0)
	err := s.JEM.DB.UpdateModelCounts(testContext, ctlId, uuid, map[params.EntityCount]int{
		params.MachineCount: 3,
		params.UnitCount:    99,
	}, t0)

	c.Assert(err, gc.Equals, nil)

	m, err := s.NewClient("bob").GetModel(&params.GetModel{
		EntityPath: modelId,
	})
	c.Assert(err, gc.Equals, nil)
	c.Assert(m.Counts, jc.DeepEquals, map[params.EntityCount]params.Count{
		params.MachineCount: {
			Time:    t0,
			Current: 3,
			Max:     3,
			Total:   3,
		},
		params.UnitCount: {
			Time:    t0,
			Current: 99,
			Max:     99,
			Total:   99,
		},
	})
}

func newTime(t time.Time) *time.Time {
	return &t
}

func (s *APISuite) openAPIFromModelResponse(c *gc.C, resp *params.ModelResponse, username string) api.Connection {
	st, err := api.Open(apiInfoFromModelResponse(resp), api.DialOpts{
		BakeryClient: s.IDMSrv.Client(username),
	})
	c.Assert(err, gc.Equals, nil)
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
			Credential: params.CredentialPath{
				Cloud: "dummy",
				User:  "beatles",
				Name:  cred,
			},
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
	c.Assert(err, gc.Equals, nil)

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
				Message: fmt.Sprintf("cannot unmarshal parameters: cannot unmarshal into field Info: cannot unmarshal request body: %s", test.expectErr),
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
	}, nil)
	s.AssertUpdateCredential(c, "bob", "dummy", "cred1", "empty")

	httptesting.AssertJSONCall(c, httptesting.JSONCallParams{
		Method:  "POST",
		URL:     "/v2/model/bob",
		Handler: s.JEMSrv,
		JSONBody: params.NewModelInfo{
			Name:       params.Name("bar"),
			Controller: &params.EntityPath{"bob", "foo"},
			Credential: params.CredentialPath{
				Cloud: "dummy",
				User:  "bob",
				Name:  "cred1",
			},
		},
		ExpectBody: params.Error{
			Message: `cannot find suitable controller`,
		},
		ExpectStatus: http.StatusInternalServerError,
		Do:           apitest.Do(s.IDMSrv.Client("bob")),
	})
}

func (s *APISuite) TestNewModelTwice(c *gc.C) {
	ctlId := s.AssertAddController(c, params.EntityPath{"bob", "foo"}, false)
	cred := s.AssertUpdateCredential(c, "bob", "dummy", "cred1", "empty")

	body := &params.NewModelInfo{
		Name:       "bar",
		Controller: &ctlId,
		Credential: params.CredentialPath{
			Cloud: "dummy",
			User:  "bob",
			Name:  cred,
		},
		Config: dummyModelConfig,
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
			Credential: params.CredentialPath{
				Cloud: "dummy",
				User:  "bob",
				Name:  cred,
			},
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
			Credential: params.CredentialPath{
				Cloud: "dummy",
				User:  "bob",
				Name:  cred,
			},
			Config: dummyModelConfig,
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
	err := s.JEM.DB.SetControllerUnavailableAt(testContext, ctlId1, unavailableTime)
	c.Assert(err, gc.Equals, nil)

	ctlId2 := s.AssertAddController(c, params.EntityPath{"bob", "another"}, false)
	err = s.JEM.DB.SetControllerUnavailableAt(testContext, ctlId2, unavailableTime.Add(time.Second))
	c.Assert(err, gc.Equals, nil)

	resp, err := s.NewClient("bob").ListController(nil)
	c.Assert(err, gc.Equals, nil)
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
	c.Assert(err, gc.Equals, nil)
	c.Assert(resp, jc.DeepEquals, &params.ListControllerResponse{})
}

func (s *APISuite) TestListControllerNoServers(c *gc.C) {
	resp, err := s.NewClient("bob").ListController(nil)
	c.Assert(err, gc.Equals, nil)
	c.Assert(resp, jc.DeepEquals, &params.ListControllerResponse{})
}

func (s *APISuite) TestListModelsNoServers(c *gc.C) {
	resp, err := s.NewClient("bob").ListModels(&params.ListModels{})
	c.Assert(err, gc.Equals, nil)
	c.Assert(resp, jc.DeepEquals, &params.ListModelsResponse{})
}

func (s *APISuite) TestListModels(c *gc.C) {
	info := s.APIInfo(c)
	ctlId0 := s.AssertAddController(c, params.EntityPath{"alice", "foo"}, false)
	aCred := s.AssertUpdateCredential(c, "alice", "dummy", "cred1", "empty")
	bCred := s.AssertUpdateCredential(c, "bob", "dummy", "cred1", "empty")
	cCred := s.AssertUpdateCredential(c, "charlie", "dummy", "cred1", "empty")
	s.allowControllerPerm(c, ctlId0)
	modelId0, uuid0 := s.CreateModel(c, params.EntityPath{"alice", "foo"}, ctlId0, aCred)
	s.allowModelPerm(c, modelId0)
	modelId1, uuid1 := s.CreateModel(c, params.EntityPath{"bob", "bar"}, ctlId0, bCred)
	modelId2, uuid2 := s.CreateModel(c, params.EntityPath{"charlie", "bar"}, ctlId0, cCred)

	// Give one of the models some counts.
	t0 := time.Unix(0, 0)
	err := s.JEM.DB.UpdateModelCounts(testContext, ctlId0, uuid1, map[params.EntityCount]int{
		params.MachineCount: 3,
	}, t0)

	c.Assert(err, gc.Equals, nil)

	resps := []params.ModelResponse{{
		Path:           modelId0,
		UUID:           uuid0,
		ControllerUUID: s.ControllerConfig.ControllerUUID(),
		CACert:         info.CACert,
		HostPorts:      info.Addrs,
		ControllerPath: ctlId0,
		Life:           "alive",
		Creator:        "alice",
	}, {
		Path:           modelId1,
		UUID:           uuid1,
		ControllerUUID: s.ControllerConfig.ControllerUUID(),
		CACert:         info.CACert,
		HostPorts:      info.Addrs,
		ControllerPath: ctlId0,
		Counts: map[params.EntityCount]params.Count{
			params.MachineCount: {
				Time:    t0,
				Current: 3,
				Max:     3,
				Total:   3,
			},
		},
		Life:    "alive",
		Creator: "bob",
	}, {
		Path:           modelId2,
		UUID:           uuid2,
		ControllerUUID: s.ControllerConfig.ControllerUUID(),
		CACert:         info.CACert,
		HostPorts:      info.Addrs,
		ControllerPath: ctlId0,
		Life:           "alive",
		Creator:        "charlie",
	}}
	tests := []struct {
		user    params.User
		all     bool
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
	}, {
		user:    "controller-admin",
		all:     true,
		indexes: []int{0, 1, 2},
	}}
	for i, test := range tests {
		c.Logf("test %d: as user %s", i, test.user)
		expectResp := &params.ListModelsResponse{
			Models: make([]params.ModelResponse, len(test.indexes)),
		}
		for i, index := range test.indexes {
			expectResp.Models[i] = resps[index]
		}

		resp, err := s.NewClient(test.user).ListModels(&params.ListModels{All: test.all})
		c.Assert(err, gc.Equals, nil)
		c.Assert(resp, jc.DeepEquals, expectResp)
	}
}

func (s *APISuite) TestListAllModelsFailsIfNotAdmin(c *gc.C) {
	resp, err := s.NewClient("bob").ListModels(&params.ListModels{All: true})
	c.Assert(err, gc.ErrorMatches, "admin access required to list all models")
	c.Assert(resp, gc.IsNil)
}

func (s *APISuite) TestGetSetControllerPerm(c *gc.C) {
	ctlId := s.AssertAddController(c, params.EntityPath{"alice", "foo"}, false)

	acl, err := s.NewClient("alice").GetControllerPerm(&params.GetControllerPerm{
		EntityPath: ctlId,
	})
	c.Assert(err, gc.Equals, nil)
	c.Assert(acl, jc.DeepEquals, params.ACL{})

	err = s.NewClient("alice").SetControllerPerm(&params.SetControllerPerm{
		EntityPath: ctlId,
		ACL: params.ACL{
			Read: []string{"a", "b"},
		},
	})
	c.Assert(err, gc.Equals, nil)
	acl, err = s.NewClient("alice").GetControllerPerm(&params.GetControllerPerm{
		EntityPath: ctlId,
	})
	c.Assert(err, gc.Equals, nil)
	c.Assert(acl, gc.DeepEquals, params.ACL{
		Read: []string{"a", "b"},
	})
}

func (s *APISuite) TestGetSetModelPerm(c *gc.C) {
	ctlId := s.AssertAddController(c, params.EntityPath{"alice", "foo"}, false)
	aCred := s.AssertUpdateCredential(c, "alice", "dummy", "cred1", "empty")
	aModel, _ := s.CreateModel(c, params.EntityPath{"alice", "foo"}, ctlId, aCred)

	acl, err := s.NewClient("alice").GetModelPerm(&params.GetModelPerm{
		EntityPath: ctlId,
	})
	c.Assert(err, gc.Equals, nil)
	c.Assert(acl, jc.DeepEquals, params.ACL{})

	err = s.NewClient("alice").SetModelPerm(&params.SetModelPerm{
		EntityPath: aModel,
		ACL: params.ACL{
			Read: []string{"a", "b"},
		},
	})
	c.Assert(err, gc.Equals, nil)
	acl, err = s.NewClient("alice").GetModelPerm(&params.GetModelPerm{
		EntityPath: aModel,
	})
	c.Assert(err, gc.Equals, nil)
	c.Assert(acl, gc.DeepEquals, params.ACL{
		Read: []string{"a", "b"},
	})
}

func (s *APISuite) TestWhoAmI(c *gc.C) {
	resp, err := s.NewClient("bob").WhoAmI(nil)
	c.Assert(err, gc.Equals, nil)
	c.Assert(resp.User, gc.Equals, "bob")
}

var mongodocAPIHostPortsTests = []struct {
	about  string
	hps    [][]network.HostPort
	expect [][]mongodoc.HostPort
}{{
	about:  "one address",
	hps:    [][]network.HostPort{{{Address: network.Address{Value: "0.1.2.3", Scope: network.ScopePublic}, Port: 1234}}},
	expect: [][]mongodoc.HostPort{{{Host: "0.1.2.3", Port: 1234, Scope: "public"}}},
}, {
	about:  "unknown scope changed to public",
	hps:    [][]network.HostPort{{{Address: network.Address{Value: "0.1.2.3", Scope: network.ScopeUnknown}, Port: 1234}}},
	expect: [][]mongodoc.HostPort{{{Host: "0.1.2.3", Port: 1234, Scope: "public"}}},
}, {
	about: "unusable addresses removed",
	hps: [][]network.HostPort{{
		{Address: network.Address{Value: "0.1.2.3", Scope: network.ScopeMachineLocal}, Port: 1234},
	}, {
		{Address: network.Address{Value: "0.1.2.4", Scope: network.ScopeLinkLocal}, Port: 1234},
		{Address: network.Address{Value: "0.1.2.5", Scope: network.ScopePublic}, Port: 1234},
	}},
	expect: [][]mongodoc.HostPort{{{Host: "0.1.2.5", Port: 1234, Scope: "public"}}},
}}

func (s *APISuite) TestMongodocAPIHostPorts(c *gc.C) {
	for i, test := range mongodocAPIHostPortsTests {
		c.Logf("test %d: %v", i, test.about)
		got := v2.MongodocAPIHostPorts(test.hps)
		c.Assert(got, jc.DeepEquals, test.expect)
	}
}

func (s *APISuite) TestJujuStatus(c *gc.C) {
	ctlId := s.AssertAddController(c, params.EntityPath{"alice", "foo"}, false)
	s.allowControllerPerm(c, ctlId)
	cred := s.AssertUpdateCredential(c, "bob", "dummy", "cred1", "empty")
	modelId, _ := s.CreateModel(c, params.EntityPath{"bob", "bar"}, ctlId, cred)

	resp, err := s.NewClient("bob").JujuStatus(&params.JujuStatus{
		EntityPath: modelId,
	})
	c.Assert(err, gc.Equals, nil)
	resp.Status.Model.ModelStatus.Since = nil
	resp.Status.ControllerTimestamp = nil
	c.Assert(resp, jc.DeepEquals, &params.JujuStatusResponse{
		Status: jujuparams.FullStatus{
			Model: jujuparams.ModelStatusInfo{
				Name:        string(modelId.Name),
				CloudTag:    names.NewCloudTag("dummy").String(),
				CloudRegion: "dummy-region",
				Version:     jujuversion.Current.String(),
				ModelStatus: jujuparams.DetailedStatus{
					Status: "available",
					Data:   make(map[string]interface{}),
				},
				SLA:  "unsupported",
				Type: "iaas",
			},
			Machines:           map[string]jujuparams.MachineStatus{},
			Applications:       map[string]jujuparams.ApplicationStatus{},
			RemoteApplications: map[string]jujuparams.RemoteApplicationStatus{},
			Relations:          nil,
			Offers:             map[string]jujuparams.ApplicationOfferStatus{},
		},
	})

	// Check that an admin can also get the status.
	resp, err = s.NewClient("alice").JujuStatus(&params.JujuStatus{
		EntityPath: modelId,
	})
	c.Assert(err, gc.Equals, nil)
	resp.Status.Model.ModelStatus.Since = nil
	resp.Status.ControllerTimestamp = nil
	c.Assert(resp, jc.DeepEquals, &params.JujuStatusResponse{
		Status: jujuparams.FullStatus{
			Model: jujuparams.ModelStatusInfo{
				Name:        string(modelId.Name),
				CloudTag:    names.NewCloudTag("dummy").String(),
				CloudRegion: "dummy-region",
				Version:     jujuversion.Current.String(),
				ModelStatus: jujuparams.DetailedStatus{
					Status: "available",
					Data:   make(map[string]interface{}),
				},
				SLA:  "unsupported",
				Type: "iaas",
			},
			Machines:           map[string]jujuparams.MachineStatus{},
			Applications:       map[string]jujuparams.ApplicationStatus{},
			RemoteApplications: map[string]jujuparams.RemoteApplicationStatus{},
			Relations:          nil,
			Offers:             map[string]jujuparams.ApplicationOfferStatus{},
		},
	})

	// Make sure another user cannot get access.
	resp, err = s.NewClient("charlie").JujuStatus(&params.JujuStatus{
		EntityPath: modelId,
	})
	c.Assert(err, gc.ErrorMatches, "unauthorized")

	// Model not found
	resp, err = s.NewClient("alice").JujuStatus(&params.JujuStatus{
		EntityPath: params.EntityPath{User: "bob", Name: "no-such-model"},
	})
	c.Assert(err, gc.ErrorMatches, `cannot get model: model "bob/no-such-model" not found`)

	resp, err = s.NewClient("bob").JujuStatus(&params.JujuStatus{
		EntityPath: params.EntityPath{User: "bob", Name: "no-such-model"},
	})
	c.Assert(err, gc.ErrorMatches, `cannot get model: model "bob/no-such-model" not found`)
}

func (s *APISuite) TestMigrate(c *gc.C) {
	ctlId1 := s.AssertAddController(c, params.EntityPath{"bob", "foo"}, true)
	// Add the controller explicitly so that we can add it
	// with an empty CACert as that matches most likely
	// production scenario.
	ctlId2 := params.EntityPath{"bob", "bar"}
	info := s.APIInfo(c)
	p := &params.AddController{
		EntityPath: ctlId2,
		Info: params.ControllerInfo{
			HostPorts:      info.Addrs,
			CACert:         info.CACert,
			User:           info.Tag.Id(),
			Password:       info.Password,
			ControllerUUID: utils.MustNewUUID().String(),
			Public:         true,
		},
	}
	err := s.NewClient("bob").AddController(p)
	c.Assert(err, gc.Equals, nil)

	s.allowControllerPerm(c, ctlId1)
	s.allowControllerPerm(c, ctlId2)

	cred := s.AssertUpdateCredential(c, "bob", "dummy", "cred1", "empty")
	modelId, _ := s.CreateModel(c, params.EntityPath{"bob", "model"}, ctlId1, cred)

	client := s.NewClient("controller-admin")

	// First check how far we get with the real InitiateMigration implementation.
	// The error signifies that we've got far enough into the migration
	// that it's contacted the target controller and found that it has
	// the same model (because it's actually the same controller
	// under the hood). This is about as decent an assurance as we
	// can get that it works without changing the juju test machinery
	// so that it can start up two controllers at the same time.
	err = client.Migrate(&params.Migrate{
		EntityPath: modelId,
		Controller: ctlId2,
	})
	c.Assert(err, gc.ErrorMatches, `cannot initiate migration: target prechecks failed: model with same UUID already exists \(.*\)`)

	// Patch out the API call and check that the controller gets changed.
	s.PatchValue(v2.ControllerClientInitiateMigration, func(*controllerapi.Client, controllerapi.MigrationSpec) (string, error) {
		return "id", nil
	})
	err = client.Migrate(&params.Migrate{
		EntityPath: modelId,
		Controller: ctlId2,
	})
	c.Assert(err, gc.Equals, nil)

	m, err := s.JEM.DB.Model(testContext, modelId)
	c.Assert(err, gc.Equals, nil)
	c.Assert(m.Controller, jc.DeepEquals, params.EntityPath{"bob", "bar"})
}

func (s *APISuite) TestLogLevel(c *gc.C) {
	c.Assert(zapctx.LogLevel.Level(), gc.Equals, zapcore.InfoLevel)
	client := s.NewClient("controller-admin")
	level, err := client.LogLevel(nil)
	c.Assert(err, gc.Equals, nil)
	c.Assert(level, jc.DeepEquals, params.Level{
		Level: "info",
	})
	err = client.SetLogLevel(&params.SetLogLevel{
		Level: params.Level{Level: "debug"},
	})
	c.Assert(err, gc.Equals, nil)
	c.Assert(zapctx.LogLevel.Level(), gc.Equals, zapcore.DebugLevel)
	level, err = client.LogLevel(nil)
	c.Assert(err, gc.Equals, nil)
	c.Assert(level, jc.DeepEquals, params.Level{
		Level: "debug",
	})
	err = client.SetLogLevel(&params.SetLogLevel{
		Level: params.Level{Level: "not-a-level"},
	})
	c.Assert(err, gc.ErrorMatches, `unrecognized level: "not-a-level"`)
	client.SetLogLevel(&params.SetLogLevel{
		Level: params.Level{Level: "info"},
	})
}

func (s *APISuite) TestGetSetControllerDeprecated(c *gc.C) {
	ctlId := s.AssertAddController(c, params.EntityPath{"alice", "foo"}, false)

	d, err := s.NewClient("alice").GetControllerDeprecated(&params.GetControllerDeprecated{
		EntityPath: ctlId,
	})
	c.Assert(err, gc.Equals, nil)
	c.Assert(d, jc.DeepEquals, &params.DeprecatedBody{
		Deprecated: false,
	})

	err = s.NewClient("alice").SetControllerDeprecated(&params.SetControllerDeprecated{
		EntityPath: ctlId,
		Body: params.DeprecatedBody{
			Deprecated: true,
		},
	})
	c.Assert(err, gc.Equals, nil)

	d, err = s.NewClient("alice").GetControllerDeprecated(&params.GetControllerDeprecated{
		EntityPath: ctlId,
	})
	c.Assert(err, gc.Equals, nil)
	c.Assert(d, jc.DeepEquals, &params.DeprecatedBody{
		Deprecated: true,
	})
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
	c.Assert(err, gc.Equals, nil)
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
	c.Assert(err, gc.Equals, nil)
}

func (s *APISuite) TestGetModelName(c *gc.C) {
	s.AssertAddController(c, params.EntityPath{"bob", "open"}, false)

	cred := s.AssertUpdateCredential(c, "bob", "dummy", "cred1", "empty")
	_, uuid := s.CreateModel(c, params.EntityPath{"bob", "open"}, params.EntityPath{"bob", "open"}, cred)

	s.allowControllerPerm(c, params.EntityPath{"bob", "open"})
	s.allowModelPerm(c, params.EntityPath{"bob", "open"})

	httptesting.AssertJSONCall(c, httptesting.JSONCallParams{
		Method:  "GET",
		Handler: s.JEMSrv,
		URL:     fmt.Sprintf("/v2/model-uuid/%v/name", uuid),
		ExpectBody: params.ModelNameResponse{
			Name: "open",
		},
		ExpectStatus: http.StatusOK,
		Do:           apitest.Do(s.IDMSrv.Client("bob")),
	})

	uuid1 := utils.MustNewUUID().String()

	httptesting.AssertJSONCall(c, httptesting.JSONCallParams{
		Method:       "GET",
		Handler:      s.JEMSrv,
		URL:          fmt.Sprintf("/v2/model-uuid/%v/name", uuid1),
		ExpectStatus: http.StatusNotFound,
		Do:           apitest.Do(s.IDMSrv.Client("bob")),
		ExpectBody: params.Error{
			Code:    "not found",
			Message: fmt.Sprintf("model %q not found", uuid1),
		},
	})

}

func (s *APISuite) TestGetAuditEntries(c *gc.C) {
	s.ACLStore.Set(testContext, "audit-log", []string{"charlie"})
	s.AssertAddController(c, params.EntityPath{"bob", "open"}, false)
	cred := s.AssertUpdateCredential(c, "bob", "dummy", "cred1", "empty")
	_, uuid := s.CreateModel(c, params.EntityPath{"bob", "open"}, params.EntityPath{"bob", "open"}, cred)

	s.allowControllerPerm(c, params.EntityPath{"bob", "open"})
	s.allowModelPerm(c, params.EntityPath{"bob", "open"})
	res, err := s.NewClient("charlie").GetAuditEntries(&params.AuditLogRequest{})
	c.Assert(err, gc.Equals, nil)
	c.Assert(res, gc.HasLen, 1)
	c.Assert(res, jemtest.CmpEquals(cmpopts.IgnoreTypes(time.Time{})), params.AuditLogEntries{{
		Content: &params.AuditModelCreated{
			ID:             "bob/open",
			UUID:           uuid,
			Owner:          "bob",
			Creator:        "bob",
			Cloud:          "dummy",
			Region:         "dummy-region",
			ControllerPath: "bob/open",
			AuditEntryCommon: params.AuditEntryCommon{
				Type_: params.AuditLogType(&params.AuditModelCreated{}),
			},
		},
	}})
}

func (s *APISuite) TestGetAuditEntriesNotAuthorized(c *gc.C) {
	httptesting.AssertJSONCall(c, httptesting.JSONCallParams{
		Method:  "GET",
		Handler: s.JEMSrv,
		URL:     "/v2/audit",
		ExpectBody: &params.Error{
			Message: `unauthorized`,
			Code:    params.ErrUnauthorized,
		},
		ExpectStatus: http.StatusUnauthorized,
		Do:           apitest.Do(s.IDMSrv.Client("alice")),
	})
}

func (s *APISuite) TestGetModelStatuses(c *gc.C) {
	s.AssertAddController(c, params.EntityPath{"bob", "open"}, false)
	cred := s.AssertUpdateCredential(c, "bob", "dummy", "cred1", "empty")
	_, uuid := s.CreateModel(c, params.EntityPath{"bob", "open"}, params.EntityPath{"bob", "open"}, cred)

	s.allowControllerPerm(c, params.EntityPath{"bob", "open"})
	s.allowModelPerm(c, params.EntityPath{"bob", "open"})
	res, err := s.NewClient("bob").GetModelStatuses(&params.ModelStatusesRequest{})
	c.Assert(err, gc.Equals, nil)
	c.Assert(res, gc.HasLen, 1)
	c.Assert(res, gc.DeepEquals, params.ModelStatuses{{
		ID:         "bob/open",
		UUID:       uuid,
		Cloud:      "dummy",
		Region:     "dummy-region",
		Status:     "available",
		Created:    res[0].Created,
		Controller: "bob/open",
	}})
}

func (s *APISuite) TestGetModelStatusesNotAuthorized(c *gc.C) {
	httptesting.AssertJSONCall(c, httptesting.JSONCallParams{
		Method:  "GET",
		Handler: s.JEMSrv,
		URL:     "/v2/modelstatus",
		ExpectBody: &params.Error{
			Message: `unauthorized`,
			Code:    params.ErrUnauthorized,
		},
		ExpectStatus: http.StatusUnauthorized,
		Do:           apitest.Do(s.IDMSrv.Client("alice")),
	})
}

func (s *APISuite) TestMissingModels(c *gc.C) {
	ctlID := s.AssertAddController(c, params.EntityPath{"bob", "open"}, false)
	cred := s.AssertUpdateCredential(c, "bob", "dummy", "cred1", "empty")
	s.CreateModel(c, params.EntityPath{"bob", "open"}, params.EntityPath{"bob", "open"}, cred)

	s.allowControllerPerm(c, params.EntityPath{"bob", "open"})
	s.allowModelPerm(c, params.EntityPath{"bob", "open"})
	res, err := s.NewClient("bob").MissingModels(&params.MissingModelsRequest{
		EntityPath: ctlID,
	})
	c.Assert(err, gc.Equals, nil)
	c.Assert(res, gc.DeepEquals, params.MissingModels{
		Models: []params.ModelStatus{{
			ID:         "admin/controller",
			UUID:       "deadbeef-0bad-400d-8000-4b1d0d06f00d",
			Cloud:      "dummy",
			Region:     "dummy-region",
			Status:     "available",
			Controller: "bob/open",
		}},
	})
}

func (s *APISuite) TestMissingModelsNotAuthorized(c *gc.C) {
	ctlID := s.AssertAddController(c, params.EntityPath{"bob", "open"}, false)
	cred := s.AssertUpdateCredential(c, "bob", "dummy", "cred1", "empty")
	s.CreateModel(c, params.EntityPath{"bob", "open"}, params.EntityPath{"bob", "open"}, cred)
	_, err := s.NewClient("alice").MissingModels(&params.MissingModelsRequest{
		EntityPath: ctlID,
	})
	c.Assert(err, gc.DeepEquals, &params.Error{
		Code:    params.ErrUnauthorized,
		Message: "admin access required",
	})
}
