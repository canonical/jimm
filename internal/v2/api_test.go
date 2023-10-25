package v2_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery/identchecker"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/juju/juju/api"
	controllerapi "github.com/juju/juju/api/controller"
	jujuparams "github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/component/all"
	jujuversion "github.com/juju/juju/version"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/testing/httptesting"
	"github.com/juju/utils/v2"
	"go.uber.org/zap/zapcore"
	gc "gopkg.in/check.v1"
	"gopkg.in/errgo.v1"

	"github.com/canonical/jimm/internal/jem/jimmdb"
	"github.com/canonical/jimm/internal/jemtest"
	"github.com/canonical/jimm/internal/jemtest/apitest"
	"github.com/canonical/jimm/internal/mongodoc"
	v2 "github.com/canonical/jimm/internal/v2"
	"github.com/canonical/jimm/internal/zapctx"
	"github.com/canonical/jimm/jemclient"
	"github.com/canonical/jimm/params"
)

func init() {
	all.RegisterForServer()
}

type APISuite struct {
	apitest.BootstrapAPISuite
}

var _ = gc.Suite(&APISuite{})

func (s *APISuite) SetUpTest(c *gc.C) {
	s.NewAPIHandler = v2.NewAPIHandler
	s.BootstrapAPISuite.SetUpTest(c)
	s.Candid.AddUser("alice", jemtest.ControllerAdmin)
}

func (s *APISuite) client(user string, groups ...string) *jemclient.Client {
	s.Candid.AddUser(user, groups...)
	return jemclient.New(jemclient.NewParams{
		BaseURL: s.HTTP.URL,
		Client:  s.Client(user),
	})
}

const sshKey = "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQDOjaOjVRHchF2RFCKQdgBqrIA5nOoqSprLK47l2th5I675jw+QYMIihXQaITss3hjrh3+5ITyBO41PS5rHLNGtlYUHX78p9CHNZsJqHl/z1Ub1tuMe+/5SY2MkDYzgfPtQtVsLasAIiht/5g78AMMXH3HeCKb9V9cP6/lPPq6mCMvg8TDLrPp/P2vlyukAsJYUvVgoaPDUBpedHbkMj07pDJqe4D7c0yEJ8hQo/6nS+3bh9Q1NvmVNsB1pbtk3RKONIiTAXYcjclmOljxxJnl1O50F5sOIi38vyl7Q63f6a3bXMvJEf1lnPNJKAxspIfEu8gRasny3FEsbHfrxEwVj rog@rog-x220"

var sampleModelConfig = map[string]interface{}{
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
	path:   "/v2/model/bob/model-1",
}, {
	about:  "get controller as non-owner",
	asUser: "other",
	method: "GET",
	path:   "/v2/controller/controller-admin/controller-1",
}, {
	about:  "new model as non-owner",
	asUser: "other",
	method: "POST",
	path:   "/v2/model/bob",
	body: params.NewModelInfo{
		Name:       "newmodel",
		Controller: &params.EntityPath{"alice", "controller-1"},
		Credential: params.CredentialPath{
			Cloud: jemtest.TestCloudName,
			User:  "bob",
			Name:  "cred",
		},
	},
}, {
	about:  "set controller perm as non-owner",
	asUser: "other",
	method: "PUT",
	path:   "/v2/controller/alice/controller-1/perm",
	body:   params.ACL{},
}, {
	about:  "set model perm as non-owner",
	asUser: "other",
	method: "PUT",
	path:   "/v2/model/bob/model-1/perm",
	body:   params.ACL{},
}, {
	about:  "get controller perm as non-owner",
	asUser: "other",
	method: "GET",
	path:   "/v2/controller/alice/controller-1/perm",
}, {
	about:  "get model perm as non-owner",
	asUser: "other",
	method: "GET",
	path:   "/v2/model/bob/model-1/perm",
}, {
	about:  "get controller perm with ACL that allows us",
	asUser: "charlie",
	method: "GET",
	path:   "/v2/controller/alice/controller-1/perm",
}, {
	about:  "get model perm with ACL that allows us",
	asUser: "charlie",
	method: "GET",
	path:   "/v2/model/bob/model-1/perm",
}, {
	about:  "set deprecated status as non-owner",
	asUser: "bob",
	method: "PUT",
	path:   "/v2/controller/alice/conntroller-1/deprecated",
	body:   params.DeprecatedBody{},
}}

func (s *APISuite) TestUnauthorized(c *gc.C) {
	ctx := context.Background()

	err := s.JEM.GrantModel(ctx, jemtest.Bob, &s.Model, "charlie", jujuparams.ModelReadAccess)
	c.Assert(err, gc.Equals, nil)
	err = s.JEM.DB.UpdateController(ctx, &s.Controller, new(jimmdb.Update).AddToSet("acl.read", "charlie"), false)
	c.Assert(err, gc.Equals, nil)

	for i, test := range unauthorizedTests {
		c.Logf("test %d: %s", i, test.about)
		httptesting.AssertJSONCall(c, httptesting.JSONCallParams{
			Method:   test.method,
			Handler:  s.APIHandler,
			JSONBody: test.body,
			URL:      test.path,
			ExpectBody: &params.Error{
				Message: `unauthorized`,
				Code:    params.ErrUnauthorized,
			},
			ExpectStatus: http.StatusUnauthorized,
			Do:           apitest.Do(s.Client(test.asUser)),
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
			Message: "validating info for opening an API connection: missing addresses not valid",
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
			Message: `unauthorized`,
		},
	}, {
		about: "cannot connect to controller",
		body: params.ControllerInfo{
			HostPorts:      []string{"127.1.2.3:1234"},
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
			c.Assert(body.Message, gc.Matches, `unable to connect to API: .*`)
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
	s.Candid.AddUser("alice", "beatles", "controller-admin")
	s.Candid.AddUser("bob", "beatles")
	s.Candid.AddUser("testuser", "controller-admin")
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
		client := s.Client(string(authUser))
		httptesting.AssertJSONCall(c, httptesting.JSONCallParams{
			Method:       "PUT",
			Handler:      s.APIHandler,
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
		ctl := &mongodoc.Controller{Path: controllerPath}
		err = s.JEM.DB.GetController(context.TODO(), ctl)
		c.Assert(err, gc.Equals, nil)
		v, ok := conn.ServerVersion()
		c.Assert(ok, gc.Equals, true)
		c.Assert(ctl.Version, jc.DeepEquals, &v)

		// Clear the connection pool for the next test.
		s.Pool.ClearAPIConnCache()
	}
}

func (s *APISuite) TestAddControllerDuplicate(c *gc.C) {
	ctx := context.Background()

	info := s.APIInfo(c)
	err := s.client("alice", jemtest.ControllerAdmin).AddController(ctx, &params.AddController{
		EntityPath: s.Controller.Path,
		Info: params.ControllerInfo{
			HostPorts:      info.Addrs,
			CACert:         info.CACert,
			User:           info.Tag.Id(),
			Password:       info.Password,
			ControllerUUID: info.ModelTag.Id(),
			Public:         true,
		},
	})
	c.Assert(err, gc.ErrorMatches, "Put http://.*/v2/controller/controller-admin/controller-1: already exists")
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrAlreadyExists)
}

func (s *APISuite) TestAddControllerUnauthenticated(c *gc.C) {
	httptesting.AssertJSONCall(c, httptesting.JSONCallParams{
		Method:   "PUT",
		Handler:  s.APIHandler,
		URL:      "/v2/controller/user/model",
		JSONBody: &params.ControllerInfo{},
		ExpectBody: httptesting.BodyAsserter(func(c *gc.C, m json.RawMessage) {
			// Allow any body - TestGetModelNotFound will check that it's a valid macaroon.
		}),
		ExpectStatus: http.StatusProxyAuthRequired,
	})
}

func (s *APISuite) TestAddControllerUnauthenticatedWithBakeryProtocol(c *gc.C) {
	httptesting.AssertJSONCall(c, httptesting.JSONCallParams{
		Method:   "PUT",
		Handler:  s.APIHandler,
		Header:   map[string][]string{"Bakery-Protocol-Version": {"1"}},
		URL:      "/v2/controller/user/model",
		JSONBody: &params.ControllerInfo{},
		ExpectBody: httptesting.BodyAsserter(func(c *gc.C, m json.RawMessage) {
			// Allow any body - TestGetModelNotFound will check that it's a valid macaroon.
		}),
		ExpectStatus: http.StatusUnauthorized,
	})
}

func (s *APISuite) TestGetModelNotFound(c *gc.C) {
	httptesting.AssertJSONCall(c, httptesting.JSONCallParams{
		Method:  "GET",
		Handler: s.APIHandler,
		URL:     "/v2/model/user/foo",
		ExpectBody: &params.Error{
			Message: `model not found`,
			Code:    params.ErrNotFound,
		},
		ExpectStatus: http.StatusNotFound,
		Do:           apitest.Do(s.Client("user")),
	})

	// If we're some different user, we get Unauthorized.
	httptesting.AssertJSONCall(c, httptesting.JSONCallParams{
		Method:  "GET",
		Handler: s.APIHandler,
		URL:     "/v2/model/user/foo",
		ExpectBody: &params.Error{
			Message: `unauthorized`,
			Code:    params.ErrUnauthorized,
		},
		ExpectStatus: http.StatusUnauthorized,
		Do:           apitest.Do(s.Client("other")),
	})
}

func (s *APISuite) TestDeleteModelNotFound(c *gc.C) {
	httptesting.AssertJSONCall(c, httptesting.JSONCallParams{
		Method:  "DELETE",
		Handler: s.APIHandler,
		URL:     "/v2/model/user/foo",
		ExpectBody: &params.Error{
			Message: `model not found`,
			Code:    params.ErrNotFound,
		},
		ExpectStatus: http.StatusNotFound,
		Do:           apitest.Do(s.Client("user")),
	})
}

func (s *APISuite) TestDeleteModel(c *gc.C) {
	resp := httptesting.DoRequest(c, httptesting.DoRequestParams{
		Handler: s.APIHandler,
		URL:     "/v2/model/" + s.Model.Path.String(),
		Do:      apitest.Do(s.Client("bob")),
	})
	c.Assert(resp.Code, gc.Equals, http.StatusOK, gc.Commentf("body: %s", resp.Body.Bytes()))

	// Delete model.
	httptesting.AssertJSONCall(c, httptesting.JSONCallParams{
		Method:       "DELETE",
		Handler:      s.APIHandler,
		URL:          "/v2/model/" + s.Model.Path.String(),
		ExpectStatus: http.StatusOK,
		Do:           apitest.Do(s.Client("bob")),
	})
	// Check that it doesn't exist anymore.
	httptesting.AssertJSONCall(c, httptesting.JSONCallParams{
		Handler:      s.APIHandler,
		URL:          "/v2/model/" + s.Model.Path.String(),
		ExpectStatus: http.StatusNotFound,
		ExpectBody: &params.Error{
			Message: `model not found`,
			Code:    params.ErrNotFound,
		},
		Do: apitest.Do(s.Client("bob")),
	})
}

func (s *APISuite) TestGetController(c *gc.C) {
	ctx := context.Background()

	t := time.Now()
	err := s.JEM.SetControllerUnavailableAt(ctx, s.Controller.Path, t)
	c.Assert(err, gc.Equals, nil)

	resp := httptesting.DoRequest(c, httptesting.DoRequestParams{
		Handler: s.APIHandler,
		URL:     "/v2/controller/" + s.Controller.Path.String(),
		Do:      apitest.Do(s.Client("alice")),
	})
	c.Assert(resp.Code, gc.Equals, http.StatusOK, gc.Commentf("body: %s", resp.Body.Bytes()))
	var controllerInfo params.ControllerResponse
	err = json.Unmarshal(resp.Body.Bytes(), &controllerInfo)
	c.Assert(err, gc.IsNil, gc.Commentf("body: %s", resp.Body.String()))
	c.Assert((*controllerInfo.UnavailableSince).UTC(), jc.DeepEquals, mongodoc.Time(t).UTC())
	c.Assert(controllerInfo.Location, jc.DeepEquals, map[string]string{
		"cloud":  jemtest.TestCloudName,
		"region": jemtest.TestCloudRegionName,
	})
	c.Logf("%#v", controllerInfo.Schema)
}

func (s *APISuite) TestGetControllerWithLocation(c *gc.C) {
	resp := httptesting.DoRequest(c, httptesting.DoRequestParams{
		Handler: s.APIHandler,
		URL:     "/v2/controller/" + s.Controller.Path.String(),
		Do:      apitest.Do(s.Client("alice")),
	})
	c.Assert(resp.Code, gc.Equals, http.StatusOK, gc.Commentf("body: %s", resp.Body.Bytes()))
	var controllerInfo params.ControllerResponse
	err := json.Unmarshal(resp.Body.Bytes(), &controllerInfo)
	c.Assert(err, gc.IsNil, gc.Commentf("body: %s", resp.Body.String()))
	c.Assert(controllerInfo.Public, gc.Equals, true)
}

func (s *APISuite) TestGetControllerNotFound(c *gc.C) {
	httptesting.AssertJSONCall(c, httptesting.JSONCallParams{
		Method:  "GET",
		Handler: s.APIHandler,
		URL:     "/v2/controller/bob/foo",
		ExpectBody: &params.Error{
			Message: `controller not found`,
			Code:    params.ErrNotFound,
		},
		ExpectStatus: http.StatusNotFound,
		Do:           apitest.Do(s.Client("bob")),
	})

	// Any other user just sees Unauthorized.
	httptesting.AssertJSONCall(c, httptesting.JSONCallParams{
		Method:  "GET",
		Handler: s.APIHandler,
		URL:     "/v2/controller/bob/foo",
		ExpectBody: &params.Error{
			Message: `unauthorized`,
			Code:    params.ErrUnauthorized,
		},
		ExpectStatus: http.StatusUnauthorized,
		Do:           apitest.Do(s.Client("alice")),
	})
}

func (s *APISuite) TestDeleteControllerNotFound(c *gc.C) {
	httptesting.AssertJSONCall(c, httptesting.JSONCallParams{
		Method:  "DELETE",
		Handler: s.APIHandler,
		URL:     "/v2/controller/bob/foobarred",
		ExpectBody: &params.Error{
			Message: `controller not found`,
			Code:    params.ErrNotFound,
		},
		ExpectStatus: http.StatusNotFound,
		Do:           apitest.Do(s.Client("bob")),
	})
}

func (s *APISuite) TestDeleteController(c *gc.C) {
	ctx := context.Background()

	// Check the controller exists.
	err := s.JEM.DB.GetController(ctx, &s.Controller)
	c.Assert(err, gc.Equals, nil)

	// Check that we can't delete it because it's marked as "available"
	httptesting.AssertJSONCall(c, httptesting.JSONCallParams{
		Method:       "DELETE",
		Handler:      s.APIHandler,
		URL:          "/v2/controller/" + s.Controller.Path.String(),
		ExpectStatus: http.StatusForbidden,
		ExpectBody: params.Error{
			Message: `cannot delete controller while it is still alive`,
			Code:    params.ErrStillAlive,
		},
		Do: apitest.Do(s.Client("alice")),
	})

	// Check that we can delete it with force flag.
	httptesting.AssertJSONCall(c, httptesting.JSONCallParams{
		Method:       "DELETE",
		Handler:      s.APIHandler,
		URL:          "/v2/controller/" + s.Controller.Path.String() + "?force=true",
		ExpectStatus: http.StatusOK,
		Do:           apitest.Do(s.Client("alice")),
	})
	// Check that it doesn't exist anymore.
	err = s.JEM.DB.GetController(ctx, &s.Controller)
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)
	// Check that the model on it doesn't exist any more either.
	err = s.JEM.DB.GetModel(ctx, &s.Model)
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)
}

func (s *APISuite) TestNewModel(c *gc.C) {
	ctx := context.Background()

	var modelRespBody json.RawMessage
	httptesting.AssertJSONCall(c, httptesting.JSONCallParams{
		Method:  "POST",
		URL:     "/v2/model/bob",
		Handler: s.APIHandler,
		JSONBody: params.NewModelInfo{
			Name:       params.Name("model-2"),
			Controller: &s.Controller.Path,
			Credential: s.Credential.Path.ToParams(),
			Location: map[string]string{
				"cloud": jemtest.TestCloudName,
			},
			Config: sampleModelConfig,
		},
		ExpectBody: httptesting.BodyAsserter(func(_ *gc.C, body json.RawMessage) {
			modelRespBody = body
		}),
		Do: apitest.Do(s.Client("bob")),
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
	modelResp2, err := s.client("bob").GetModel(ctx, &params.GetModel{
		EntityPath: params.EntityPath{
			User: "bob",
			Name: "model-2",
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
	user:  "bob",
	info: params.NewModelInfo{
		Name: "model-2",
		Credential: params.CredentialPath{
			Cloud: jemtest.TestCloudName,
			User:  "bob",
			Name:  "cred",
		},
		Location: map[string]string{
			"cloud": jemtest.TestCloudName,
		},
		Config: map[string]interface{}{
			"secret": "a secret",
		},
	},
}, {
	about: "no matching cloud",
	user:  "bob",
	info: params.NewModelInfo{
		Name: "model-3",
		Credential: params.CredentialPath{
			Cloud: jemtest.TestCloudName,
			User:  "bob",
			Name:  "cred",
		},
		Location: map[string]string{
			"cloud": "aws",
		},
		Config: map[string]interface{}{
			"secret": "a secret",
		},
	},
	expectError:      `Post http://.*/v2/model/bob: cloudregion not found`,
	expectErrorCause: params.ErrNotFound,
}, {
	about: "no matching region",
	user:  "bob",
	info: params.NewModelInfo{
		Name: "test-model",
		Credential: params.CredentialPath{
			Cloud: jemtest.TestCloudName,
			User:  "bob",
			Name:  "cred",
		},
		Location: map[string]string{
			"region": "us-east-1",
		},
		Config: map[string]interface{}{
			"secret": "a secret",
		},
	},
	expectError:      `Post http://.*/v2/model/bob: cloudregion not found`,
	expectErrorCause: params.ErrNotFound,
}, {
	about: "unrecognised location parameter",
	user:  "bob",
	info: params.NewModelInfo{
		Name: "model-3",
		Credential: params.CredentialPath{
			Cloud: jemtest.TestCloudName,
			User:  "bob",
			Name:  "cred",
		},
		Location: map[string]string{
			"dimension": "5th",
		},
		Config: map[string]interface{}{
			"secret": "a secret",
		},
	},
	expectError:      `Post http://.*/v2/model/bob: cannot select controller: no matching controllers found`,
	expectErrorCause: params.ErrNotFound,
}, {
	about: "invalid location parameter",
	user:  "bob",
	info: params.NewModelInfo{
		Name: "test-model",
		Credential: params.CredentialPath{
			Cloud: jemtest.TestCloudName,
			User:  "bob",
			Name:  "cred",
		},
		Location: map[string]string{
			"cloud.blah": jemtest.TestCloudName,
		},
		Config: map[string]interface{}{
			"secret": "a secret",
		},
	},
	expectError:      `Post http://.*/v2/model/bob: cannot select controller: no matching controllers found`,
	expectErrorCause: params.ErrNotFound,
}, {
	about: "invalid cloud name",
	user:  "bob",
	info: params.NewModelInfo{
		Name: "model-3",
		Credential: params.CredentialPath{
			Cloud: jemtest.TestCloudName,
			User:  "bob",
			Name:  "cred",
		},
		Location: map[string]string{
			"cloud": "bad/name",
		},
		Config: map[string]interface{}{
			"secret": "a secret",
		},
	},
	expectError:      `Post http://.*/v2/model/bob: invalid cloud "bad/name"`,
	expectErrorCause: params.ErrBadRequest,
}}

func (s *APISuite) TestNewModelWithoutExplicitController(c *gc.C) {
	ctx := context.Background()

	for i, test := range newModelWithoutExplicitControllerTests {
		c.Logf("test %d. %s", i, test.about)
		test.info.Name = params.Name(fmt.Sprintf("test-model-%d", i))
		resp, err := s.client(string(test.user)).NewModel(ctx, &params.NewModel{
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
		c.Assert(resp.ControllerPath, jc.DeepEquals, s.Controller.Path)
	}
}

func (s *APISuite) assertModelConfigAttr(c *gc.C, modelPath params.EntityPath, attr string, val interface{}) {
	ctx := context.Background()

	m := mongodoc.Model{Path: modelPath}
	err := s.JEM.DB.GetModel(ctx, &m)
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
	ctx := context.Background()

	err := s.JEM.SetModelLife(ctx, s.Controller.Path, s.Model.UUID, "dying")
	c.Assert(err, gc.Equals, nil)
	t := time.Now()
	err = s.JEM.SetControllerUnavailableAt(ctx, s.Controller.Path, t)
	c.Assert(err, gc.Equals, nil)

	var modelRespBody json.RawMessage
	httptesting.AssertJSONCall(c, httptesting.JSONCallParams{
		Method:  "GET",
		Handler: s.APIHandler,
		URL:     "/v2/model/" + s.Model.Path.String(),
		ExpectBody: httptesting.BodyAsserter(func(_ *gc.C, body json.RawMessage) {
			modelRespBody = body
		}),
		Do: apitest.Do(s.Client("bob")),
	})
	var modelResp params.ModelResponse
	err = json.Unmarshal(modelRespBody, &modelResp)
	c.Assert(err, gc.Equals, nil)
	c.Assert(modelResp, jc.DeepEquals, params.ModelResponse{
		Path:             s.Model.Path,
		UUID:             s.Model.UUID,
		ControllerPath:   s.Model.Controller,
		ControllerUUID:   s.Controller.UUID,
		CACert:           s.Controller.CACert,
		HostPorts:        s.APIInfo(c).Addrs,
		Life:             "dying",
		UnavailableSince: newTime(mongodoc.Time(t).UTC()),
		Creator:          "bob",
	})
}

func (s *APISuite) TestGetModelWithCounts(c *gc.C) {
	ctx := context.Background()

	t0 := time.Unix(0, 0)
	err := s.JEM.UpdateModelCounts(ctx, s.Controller.Path, s.Model.UUID, map[params.EntityCount]int{
		params.MachineCount: 3,
		params.UnitCount:    99,
	}, t0)

	c.Assert(err, gc.Equals, nil)

	m, err := s.client("bob").GetModel(ctx, &params.GetModel{
		EntityPath: s.Model.Path,
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
		BakeryClient: s.Client(username),
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
	s.Candid.AddUser("bob", "beatles")
	cred := jemtest.EmptyCredential("beatles", "cred")
	s.UpdateCredential(c, &cred)

	var modelRespBody json.RawMessage
	httptesting.AssertJSONCall(c, httptesting.JSONCallParams{
		Method:  "POST",
		URL:     "/v2/model/beatles",
		Handler: s.APIHandler,
		JSONBody: params.NewModelInfo{
			Name:       params.Name("bar"),
			Controller: &s.Controller.Path,
			Credential: params.CredentialPath{
				Cloud: jemtest.TestCloudName,
				User:  "beatles",
				Name:  "cred",
			},
			Location: map[string]string{
				"cloud": jemtest.TestCloudName,
			},
			Config: sampleModelConfig,
		},
		ExpectBody: httptesting.BodyAsserter(func(_ *gc.C, body json.RawMessage) {
			modelRespBody = body
		}),
		Do: apitest.Do(s.Client("bob")),
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
			Handler: s.APIHandler,
			JSONBody: map[string]interface{}{
				"name":       "bar",
				"controller": test.path,
			},
			ExpectBody: params.Error{
				Message: fmt.Sprintf("cannot unmarshal parameters: cannot unmarshal into field Info: cannot unmarshal request body: %s", test.expectErr),
				Code:    params.ErrBadRequest,
			},
			ExpectStatus: http.StatusBadRequest,
			Do:           apitest.Do(s.Client("bob")),
		})
	}
}

func (s *APISuite) TestNewModelCannotOpenAPI(c *gc.C) {
	ctx := context.Background()

	s.JEM.DB.UpdateController(ctx, &s.Controller, new(jimmdb.Update).Unset("adminpassword"), false)
	s.Pool.ClearAPIConnCache()

	httptesting.AssertJSONCall(c, httptesting.JSONCallParams{
		Method:  "POST",
		URL:     "/v2/model/bob",
		Handler: s.APIHandler,
		JSONBody: params.NewModelInfo{
			Name:       "model-2",
			Controller: &s.Controller.Path,
			Credential: s.Credential.Path.ToParams(),
		},
		ExpectBody: params.Error{
			Message: `cannot create model: cannot connect to controller: interaction required but not possible`,
		},
		ExpectStatus: http.StatusInternalServerError,
		Do:           apitest.Do(s.Client("bob")),
	})
}

func (s *APISuite) TestNewModelAlreadyExists(c *gc.C) {
	body := &params.NewModelInfo{
		Name:       s.Model.Path.Name,
		Controller: &s.Model.Controller,
		Credential: s.Model.Credential.ToParams(),
		Config:     sampleModelConfig,
	}
	p := httptesting.JSONCallParams{
		Method:   "POST",
		URL:      "/v2/model/bob",
		Handler:  s.APIHandler,
		JSONBody: body,
		ExpectBody: params.Error{
			Code:    params.ErrAlreadyExists,
			Message: "already exists",
		},
		ExpectStatus: http.StatusForbidden,
		Do:           apitest.Do(s.Client("bob")),
	}
	httptesting.AssertJSONCall(c, p)
}

func (s *APISuite) TestNewModelCannotCreate(c *gc.C) {
	httptesting.AssertJSONCall(c, httptesting.JSONCallParams{
		Method:  "POST",
		URL:     "/v2/model/bob",
		Handler: s.APIHandler,
		JSONBody: params.NewModelInfo{
			Name:       "bar",
			Controller: &s.Controller.Path,
			Credential: s.Credential.Path.ToParams(),
			Config: map[string]interface{}{
				"authorized-keys": sshKey,
				"logging-config":  "bad>",
			},
		},
		ExpectBody: params.Error{
			Message: `cannot create model: api error: failed to create config: creating config from values failed: config value expected '=', found "bad>"`,
		},
		ExpectStatus: http.StatusInternalServerError,
		Do:           apitest.Do(s.Client("bob")),
	})

	// Check that the model is not there (it was added temporarily during the call).
	httptesting.AssertJSONCall(c, httptesting.JSONCallParams{
		Method:  "GET",
		Handler: s.APIHandler,
		URL:     "/v2/model/bob/bar",
		ExpectBody: &params.Error{
			Message: `model not found`,
			Code:    params.ErrNotFound,
		},
		ExpectStatus: http.StatusNotFound,
		Do:           apitest.Do(s.Client("bob")),
	})
}

func (s *APISuite) TestNewModelUnauthorized(c *gc.C) {
	httptesting.AssertJSONCall(c, httptesting.JSONCallParams{
		Method:  "POST",
		URL:     "/v2/model/bob",
		Handler: s.APIHandler,
		JSONBody: params.NewModelInfo{
			Name:       "bar",
			Controller: &s.Controller.Path,
			Credential: s.Credential.Path.ToParams(),
			Config:     sampleModelConfig,
		},
		ExpectBody: params.Error{
			Message: `unauthorized`,
			Code:    params.ErrUnauthorized,
		},
		ExpectStatus: http.StatusUnauthorized,
		Do:           apitest.Do(s.Client("other")),
	})
}

func (s *APISuite) TestListController(c *gc.C) {
	ctx := context.Background()

	c0 := mongodoc.Controller{Path: params.EntityPath{jemtest.ControllerAdmin, "controller-0"}}
	s.AddController(c, &c0)
	unavailableTime := time.Now()
	err := s.JEM.SetControllerUnavailableAt(ctx, s.Controller.Path, unavailableTime)
	c.Assert(err, gc.Equals, nil)
	c2 := mongodoc.Controller{Path: params.EntityPath{jemtest.ControllerAdmin, "controller-2"}}
	s.AddController(c, &c2)
	err = s.JEM.SetControllerUnavailableAt(ctx, c2.Path, unavailableTime.Add(time.Second))
	c.Assert(err, gc.Equals, nil)

	resp, err := s.client("alice").ListController(ctx, nil)
	c.Assert(err, gc.Equals, nil)
	c.Assert(resp, jc.DeepEquals, &params.ListControllerResponse{
		Controllers: []params.ControllerResponse{{
			Path:     c0.Path,
			Location: map[string]string{"cloud": jemtest.TestCloudName, "region": jemtest.TestCloudRegionName},
			Public:   true,
		}, {
			Path:             s.Controller.Path,
			Location:         map[string]string{"cloud": jemtest.TestCloudName, "region": jemtest.TestCloudRegionName},
			UnavailableSince: newTime(mongodoc.Time(unavailableTime).UTC()),
			Public:           true,
		}, {
			Path:             c2.Path,
			Location:         map[string]string{"cloud": jemtest.TestCloudName, "region": jemtest.TestCloudRegionName},
			UnavailableSince: newTime(mongodoc.Time(unavailableTime.Add(time.Second)).UTC()),
			Public:           true,
		}},
	})

	// Check that the private entries don't show up when listing
	// as a different user.
	resp, err = s.client("bob").ListController(ctx, nil)
	c.Assert(err, gc.Equals, nil)
	c.Assert(resp, jc.DeepEquals, &params.ListControllerResponse{})
}

func (s *APISuite) TestListControllerNoServers(c *gc.C) {
	ctx := context.Background()

	err := s.client("alice").DeleteController(ctx, &params.DeleteController{
		EntityPath: s.Controller.Path,
		Force:      true,
	})
	c.Assert(err, gc.Equals, nil)

	resp, err := s.client("alice").ListController(ctx, nil)
	c.Assert(err, gc.Equals, nil)
	c.Assert(resp, jc.DeepEquals, &params.ListControllerResponse{})
}

func (s *APISuite) TestListModelsNoServers(c *gc.C) {
	ctx := context.Background()

	err := s.client("alice").DeleteController(ctx, &params.DeleteController{
		EntityPath: s.Controller.Path,
		Force:      true,
	})
	c.Assert(err, gc.Equals, nil)

	resp, err := s.client("bob").ListModels(ctx, &params.ListModels{})
	c.Assert(err, gc.Equals, nil)
	c.Assert(resp, jc.DeepEquals, &params.ListModelsResponse{})
}

func (s *APISuite) TestListModels(c *gc.C) {
	ctx := context.Background()

	info := s.APIInfo(c)
	aCred := jemtest.EmptyCredential("alice", "cred")
	s.UpdateCredential(c, &aCred)
	cCred := jemtest.EmptyCredential("charlie", "cred")
	s.UpdateCredential(c, &cCred)

	aModel := mongodoc.Model{
		Path:       params.EntityPath{"alice", "model-1"},
		Credential: aCred.Path,
	}
	s.CreateModel(c, &aModel, nil, map[params.User]jujuparams.UserAccessPermission{identchecker.Everyone: jujuparams.ModelReadAccess})
	cModel := mongodoc.Model{
		Path:       params.EntityPath{"charlie", "model-1"},
		Credential: cCred.Path,
	}
	s.CreateModel(c, &cModel, nil, nil)

	// Give one of the models some counts.
	t0 := time.Unix(0, 0)
	err := s.JEM.UpdateModelCounts(ctx, s.Controller.Path, s.Model.UUID, map[params.EntityCount]int{
		params.MachineCount: 3,
	}, t0)

	c.Assert(err, gc.Equals, nil)

	resps := []params.ModelResponse{{
		Path:           aModel.Path,
		UUID:           aModel.UUID,
		ControllerUUID: s.Controller.UUID,
		CACert:         info.CACert,
		HostPorts:      info.Addrs,
		ControllerPath: s.Controller.Path,
		Life:           "alive",
		Creator:        "alice",
	}, {
		Path:           s.Model.Path,
		UUID:           s.Model.UUID,
		ControllerUUID: s.Controller.UUID,
		CACert:         info.CACert,
		HostPorts:      info.Addrs,
		ControllerPath: s.Controller.Path,
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
		Path:           cModel.Path,
		UUID:           cModel.UUID,
		ControllerUUID: s.Controller.UUID,
		CACert:         info.CACert,
		HostPorts:      info.Addrs,
		ControllerPath: s.Controller.Path,
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

		resp, err := s.client(string(test.user)).ListModels(ctx, &params.ListModels{All: test.all})
		c.Assert(err, gc.Equals, nil)
		c.Assert(resp, jc.DeepEquals, expectResp)
	}
}

func (s *APISuite) TestListAllModelsFailsIfNotAdmin(c *gc.C) {
	ctx := context.Background()

	resp, err := s.client("bob").ListModels(ctx, &params.ListModels{All: true})
	c.Assert(err, gc.ErrorMatches, `Get http://.*/v2/model\?all=true: admin access required to list all models`)
	c.Assert(resp, gc.IsNil)
}

func (s *APISuite) TestGetSetControllerPerm(c *gc.C) {
	ctx := context.Background()

	acl, err := s.client("alice").GetControllerPerm(ctx, &params.GetControllerPerm{
		EntityPath: s.Controller.Path,
	})
	c.Assert(err, gc.Equals, nil)
	c.Assert(acl, jc.DeepEquals, params.ACL{})

	err = s.client("alice").SetControllerPerm(ctx, &params.SetControllerPerm{
		EntityPath: s.Controller.Path,
		ACL: params.ACL{
			Read: []string{"a", "b"},
		},
	})
	c.Assert(err, gc.Equals, nil)
	acl, err = s.client("alice").GetControllerPerm(ctx, &params.GetControllerPerm{
		EntityPath: s.Controller.Path,
	})
	c.Assert(err, gc.Equals, nil)
	c.Assert(acl, gc.DeepEquals, params.ACL{
		Read: []string{"a", "b"},
	})
}

func (s *APISuite) TestGetSetModelPerm(c *gc.C) {
	ctx := context.Background()

	acl, err := s.client("bob").GetModelPerm(ctx, &params.GetModelPerm{
		EntityPath: s.Model.Path,
	})
	c.Assert(err, gc.Equals, nil)
	c.Assert(acl, jc.DeepEquals, params.ACL{})

	err = s.client("bob").SetModelPerm(ctx, &params.SetModelPerm{
		EntityPath: s.Model.Path,
		ACL: params.ACL{
			Read: []string{"a", "b"},
		},
	})
	c.Assert(err, gc.Equals, nil)
	acl, err = s.client("bob").GetModelPerm(ctx, &params.GetModelPerm{
		EntityPath: s.Model.Path,
	})
	c.Assert(err, gc.Equals, nil)
	c.Assert(acl, gc.DeepEquals, params.ACL{
		Read: []string{"a", "b"},
	})
}

func (s *APISuite) TestWhoAmI(c *gc.C) {
	ctx := context.Background()

	resp, err := s.client("bob").WhoAmI(ctx, nil)
	c.Assert(err, gc.Equals, nil)
	c.Assert(resp.User, gc.Equals, "bob")
}

func (s *APISuite) TestJujuStatus(c *gc.C) {
	ctx := context.Background()

	resp, err := s.client("bob").JujuStatus(ctx, &params.JujuStatus{
		EntityPath: s.Model.Path,
	})
	c.Assert(err, gc.Equals, nil)
	resp.Status.Model.ModelStatus.Since = nil
	resp.Status.ControllerTimestamp = nil
	c.Assert(resp, jc.DeepEquals, &params.JujuStatusResponse{
		Status: jujuparams.FullStatus{
			Model: jujuparams.ModelStatusInfo{
				Name:        string(s.Model.Path.Name),
				CloudTag:    names.NewCloudTag(jemtest.TestCloudName).String(),
				CloudRegion: jemtest.TestCloudRegionName,
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
			Branches:           map[string]jujuparams.BranchStatus{},
		},
	})

	// Check that an admin can also get the status.
	resp, err = s.client("alice").JujuStatus(ctx, &params.JujuStatus{
		EntityPath: s.Model.Path,
	})
	c.Assert(err, gc.Equals, nil)
	resp.Status.Model.ModelStatus.Since = nil
	resp.Status.ControllerTimestamp = nil
	c.Assert(resp, jc.DeepEquals, &params.JujuStatusResponse{
		Status: jujuparams.FullStatus{
			Model: jujuparams.ModelStatusInfo{
				Name:        string(s.Model.Path.Name),
				CloudTag:    names.NewCloudTag(jemtest.TestCloudName).String(),
				CloudRegion: jemtest.TestCloudRegionName,
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
			Branches:           map[string]jujuparams.BranchStatus{},
		},
	})

	// Make sure another user cannot get access.
	resp, err = s.client("charlie").JujuStatus(ctx, &params.JujuStatus{
		EntityPath: s.Model.Path,
	})
	c.Assert(err, gc.ErrorMatches, "Get http://.*/v2/model/bob/model-1/status: unauthorized")

	// Model not found
	resp, err = s.client("alice").JujuStatus(ctx, &params.JujuStatus{
		EntityPath: params.EntityPath{User: "bob", Name: "no-such-model"},
	})
	c.Assert(err, gc.ErrorMatches, `Get http://.*/v2/model/bob/no-such-model/status: model not found`)

	resp, err = s.client("bob").JujuStatus(ctx, &params.JujuStatus{
		EntityPath: params.EntityPath{User: "bob", Name: "no-such-model"},
	})
	c.Assert(err, gc.ErrorMatches, `Get http://.*/v2/model/bob/no-such-model/status: model not found`)
}

func (s *APISuite) TestMigrate(c *gc.C) {
	ctx := context.Background()

	ctl2 := mongodoc.Controller{Path: params.EntityPath{User: "alice", Name: "controller-2"}}
	s.AddController(c, &ctl2)

	client := s.client(jemtest.ControllerAdmin)
	// First check how far we get with the real InitiateMigration implementation.
	// The error signifies that we've got far enough into the migration
	// that it's contacted the target controller and found that it has
	// the same model (because it's actually the same controller
	// under the hood). This is about as decent an assurance as we
	// can get that it works without changing the juju test machinery
	// so that it can start up two controllers at the same time.
	err := client.Migrate(ctx, &params.Migrate{
		EntityPath: s.Model.Path,
		Controller: ctl2.Path,
	})
	c.Assert(err, gc.ErrorMatches, `Post http://.*/v2/model/bob/model-1/migrate\?controller=alice%2Fcontroller-2: cannot initiate migration: target prechecks failed: model with same UUID already exists \(.*\)`)

	// Patch out the API call and check that the controller gets changed.
	s.PatchValue(v2.ControllerClientInitiateMigration, func(*controllerapi.Client, controllerapi.MigrationSpec) (string, error) {
		return "id", nil
	})
	err = client.Migrate(ctx, &params.Migrate{
		EntityPath: s.Model.Path,
		Controller: ctl2.Path,
	})
	c.Assert(err, gc.Equals, nil)

	m := mongodoc.Model{Path: s.Model.Path}
	err = s.JEM.DB.GetModel(ctx, &m)
	c.Assert(err, gc.Equals, nil)
	c.Assert(m.Controller, jc.DeepEquals, ctl2.Path)
}

func (s *APISuite) TestLogLevel(c *gc.C) {
	ctx := context.Background()

	c.Assert(zapctx.LogLevel.Level(), gc.Equals, zapcore.InfoLevel)
	client := s.client(jemtest.ControllerAdmin)
	level, err := client.LogLevel(ctx, nil)
	c.Assert(err, gc.Equals, nil)
	c.Assert(level, jc.DeepEquals, params.Level{
		Level: "info",
	})
	err = client.SetLogLevel(ctx, &params.SetLogLevel{
		Level: params.Level{Level: "debug"},
	})
	c.Assert(err, gc.Equals, nil)
	c.Assert(zapctx.LogLevel.Level(), gc.Equals, zapcore.DebugLevel)
	level, err = client.LogLevel(ctx, nil)
	c.Assert(err, gc.Equals, nil)
	c.Assert(level, jc.DeepEquals, params.Level{
		Level: "debug",
	})
	err = client.SetLogLevel(ctx, &params.SetLogLevel{
		Level: params.Level{Level: "not-a-level"},
	})
	c.Assert(err, gc.ErrorMatches, `Put http://.*/v2/log-level: unrecognized level: "not-a-level"`)
	client.SetLogLevel(ctx, &params.SetLogLevel{
		Level: params.Level{Level: "info"},
	})
}

func (s *APISuite) TestGetSetControllerDeprecated(c *gc.C) {
	ctx := context.Background()

	d, err := s.client("alice").GetControllerDeprecated(ctx, &params.GetControllerDeprecated{
		EntityPath: s.Controller.Path,
	})
	c.Assert(err, gc.Equals, nil)
	c.Assert(d, jc.DeepEquals, &params.DeprecatedBody{
		Deprecated: false,
	})

	err = s.client("alice").SetControllerDeprecated(ctx, &params.SetControllerDeprecated{
		EntityPath: s.Controller.Path,
		Body: params.DeprecatedBody{
			Deprecated: true,
		},
	})
	c.Assert(err, gc.Equals, nil)

	d, err = s.client("alice").GetControllerDeprecated(ctx, &params.GetControllerDeprecated{
		EntityPath: s.Controller.Path,
	})
	c.Assert(err, gc.Equals, nil)
	c.Assert(d, jc.DeepEquals, &params.DeprecatedBody{
		Deprecated: true,
	})
}

func (s *APISuite) allowControllerPerm(ctx context.Context, c *gc.C, path params.EntityPath, acl ...string) {
	if len(acl) == 0 {
		acl = []string{"everyone"}
	}
	err := s.client(string(path.User)).SetControllerPerm(ctx, &params.SetControllerPerm{
		EntityPath: path,
		ACL: params.ACL{
			Read: acl,
		},
	})
	c.Assert(err, gc.Equals, nil)
}

func (s *APISuite) allowModelPerm(ctx context.Context, c *gc.C, path params.EntityPath, acl ...string) {
	if len(acl) == 0 {
		acl = []string{"everyone"}
	}
	err := s.client(string(path.User)).SetModelPerm(ctx, &params.SetModelPerm{
		EntityPath: path,
		ACL: params.ACL{
			Read: acl,
		},
	})
	c.Assert(err, gc.Equals, nil)
}

func (s *APISuite) TestGetModelName(c *gc.C) {
	httptesting.AssertJSONCall(c, httptesting.JSONCallParams{
		Method:  "GET",
		Handler: s.APIHandler,
		URL:     fmt.Sprintf("/v2/model-uuid/%v/name", s.Model.UUID),
		ExpectBody: params.ModelNameResponse{
			Name: string(s.Model.Path.Name),
		},
		ExpectStatus: http.StatusOK,
		Do:           apitest.Do(s.Client("bob")),
	})

	uuid1 := utils.MustNewUUID().String()

	httptesting.AssertJSONCall(c, httptesting.JSONCallParams{
		Method:       "GET",
		Handler:      s.APIHandler,
		URL:          fmt.Sprintf("/v2/model-uuid/%v/name", uuid1),
		ExpectStatus: http.StatusNotFound,
		Do:           apitest.Do(s.Client("bob")),
		ExpectBody: params.Error{
			Code:    "not found",
			Message: fmt.Sprintf("model not found"),
		},
	})
}

func (s *APISuite) TestGetAuditEntries(c *gc.C) {
	ctx := context.Background()

	s.ACLStore.Set(ctx, "audit-log", []string{"charlie"})
	res, err := s.client("charlie").GetAuditEntries(ctx, &params.AuditLogRequest{})
	c.Assert(err, gc.Equals, nil)
	c.Assert(res, gc.HasLen, 1)
	c.Assert(res, jemtest.CmpEquals(cmpopts.IgnoreTypes(time.Time{})), params.AuditLogEntries{{
		Content: &params.AuditModelCreated{
			ID:             s.Model.Path.String(),
			UUID:           s.Model.UUID,
			Owner:          "bob",
			Creator:        "bob",
			Cloud:          jemtest.TestCloudName,
			Region:         jemtest.TestCloudRegionName,
			ControllerPath: s.Controller.Path.String(),
			AuditEntryCommon: params.AuditEntryCommon{
				Type_: params.AuditLogType(&params.AuditModelCreated{}),
			},
		},
	}})
}

func (s *APISuite) TestGetAuditEntriesNotAuthorized(c *gc.C) {
	httptesting.AssertJSONCall(c, httptesting.JSONCallParams{
		Method:  "GET",
		Handler: s.APIHandler,
		URL:     "/v2/audit",
		ExpectBody: &params.Error{
			Message: `unauthorized`,
			Code:    params.ErrUnauthorized,
		},
		ExpectStatus: http.StatusUnauthorized,
		Do:           apitest.Do(s.Client("charlie")),
	})
}

func (s *APISuite) TestGetModelStatuses(c *gc.C) {
	ctx := context.Background()

	res, err := s.client("alice").GetModelStatuses(ctx, &params.ModelStatusesRequest{})
	c.Assert(err, gc.Equals, nil)
	c.Assert(res, gc.HasLen, 1)
	c.Assert(res, gc.DeepEquals, params.ModelStatuses{{
		ID:         s.Model.Path.String(),
		UUID:       s.Model.UUID,
		Cloud:      jemtest.TestCloudName,
		Region:     jemtest.TestCloudRegionName,
		Status:     "available",
		Created:    res[0].Created,
		Controller: s.Controller.Path.String(),
	}})
}

func (s *APISuite) TestGetModelStatusesNotAuthorized(c *gc.C) {
	httptesting.AssertJSONCall(c, httptesting.JSONCallParams{
		Method:  "GET",
		Handler: s.APIHandler,
		URL:     "/v2/modelstatus",
		ExpectBody: &params.Error{
			Message: `unauthorized`,
			Code:    params.ErrUnauthorized,
		},
		ExpectStatus: http.StatusUnauthorized,
		Do:           apitest.Do(s.Client("bob")),
	})
}

func (s *APISuite) TestMissingModels(c *gc.C) {
	ctx := context.Background()

	res, err := s.client("alice").MissingModels(ctx, &params.MissingModelsRequest{
		EntityPath: s.Controller.Path,
	})
	c.Assert(err, gc.Equals, nil)
	c.Assert(res, gc.DeepEquals, params.MissingModels{
		Models: []params.ModelStatus{{
			ID:         "admin/controller",
			UUID:       "deadbeef-0bad-400d-8000-4b1d0d06f00d",
			Cloud:      jemtest.TestCloudName,
			Region:     jemtest.TestCloudRegionName,
			Status:     "available",
			Controller: jemtest.ControllerAdmin + "/controller-1",
		}},
	})
}

func (s *APISuite) TestMissingModelsNotAuthorized(c *gc.C) {
	ctx := context.Background()

	_, err := s.client("bob").MissingModels(ctx, &params.MissingModelsRequest{
		EntityPath: s.Controller.Path,
	})
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrUnauthorized)
	c.Assert(err, gc.ErrorMatches, `Get http://.*/v2/controller/controller-admin/controller-1/missing-models: admin access required`)
}
