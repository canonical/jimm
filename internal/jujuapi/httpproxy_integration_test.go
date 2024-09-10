// Copyright 2024 Canonical.

package jujuapi_test

import (
	"context"
	"database/sql"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"

	"github.com/juju/charm/v12"
	"github.com/juju/charm/v12/resource"
	"github.com/juju/juju/api"
	"github.com/juju/juju/api/client/charms"
	"github.com/juju/juju/api/client/resources"
	"github.com/juju/juju/testcharms"
	"github.com/juju/juju/testing/factory"
	"github.com/juju/names/v5"
	"github.com/juju/version/v2"
	gc "gopkg.in/check.v1"

	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/jimmtest"
	"github.com/canonical/jimm/v3/internal/jujuapi"
	ofganames "github.com/canonical/jimm/v3/internal/openfga/names"
)

type httpProxySuite struct {
	jimmtest.JIMMSuite
	model *dbmodel.Model
}

const testEnv = `
clouds:
- name: test-cloud
  type: test-provider
  regions:
  - name: test-cloud-region
cloud-credentials:
- owner: alice@canonical.com
  name: cred-1
  cloud: test-cloud
controllers:
- name: controller-1
  uuid: 00000001-0000-0000-0000-000000000001
  cloud: test-cloud
  region: test-cloud-region
models:
- name: model-1
  uuid: 00000002-0000-0000-0000-000000000001
  controller: controller-1
  cloud: test-cloud
  region: test-cloud-region
  cloud-credential: cred-1
  owner: alice@canonical.com
users:
- username: alice@canonical.com
  access: admin
`

var _ = gc.Suite(&httpProxySuite{})

func (s *httpProxySuite) SetUpTest(c *gc.C) {
	s.JIMMSuite.SetUpTest(c)
	ctx := context.Background()
	tester := jimmtest.GocheckTester{C: c}
	env := jimmtest.ParseEnvironment(tester, testEnv)
	env.PopulateDB(tester, s.JIMM.Database)
	user, err := s.JIMM.FetchIdentity(ctx, env.Users[0].Username)
	c.Assert(err, gc.IsNil)
	err = user.SetModelAccess(ctx, names.NewModelTag(env.Models[0].UUID), ofganames.AdministratorRelation)
	c.Assert(err, gc.IsNil)
	model := &dbmodel.Model{UUID: sql.NullString{String: env.Models[0].UUID, Valid: true}}
	err = s.JIMM.Database.GetModel(ctx, model)
	c.Assert(err, gc.IsNil)
	s.model = model
	err = s.JIMM.GetCredentialStore().PutControllerCredentials(ctx, model.Controller.Name, "user", "psw")
	c.Assert(err, gc.IsNil)
}

func (s *httpProxySuite) TestHTTPAuthenticate(c *gc.C) {
	ctx := context.Background()
	httpProxier := jujuapi.HTTPProxier(s.JIMM)

	// good
	req, err := http.NewRequest("POST", fmt.Sprintf("/%s/charms", s.model.UUID.String), nil)
	c.Assert(err, gc.IsNil)
	token, err := s.JIMM.OAuthAuthenticator.MintSessionToken("alice@canonical.com")
	c.Assert(err, gc.IsNil)
	req.SetBasicAuth("", token)
	c.Assert(err, gc.IsNil)
	err = httpProxier.Authenticate(ctx, nil, req)
	c.Assert(err, gc.IsNil)

	// missing auth
	req, err = http.NewRequest("POST", fmt.Sprintf("/%s/charms", s.model.UUID.String), nil)
	c.Assert(err, gc.IsNil)
	err = httpProxier.Authenticate(ctx, nil, req)
	c.Assert(err, gc.ErrorMatches, "authentication missing")

	// wrong user
	req, err = http.NewRequest("POST", fmt.Sprintf("/%s/charms", s.model.UUID.String), nil)
	c.Assert(err, gc.IsNil)
	token, err = s.JIMM.OAuthAuthenticator.MintSessionToken("test-user")
	c.Assert(err, gc.IsNil)
	req.SetBasicAuth("", token)
	c.Assert(err, gc.IsNil)
	err = httpProxier.Authenticate(ctx, nil, req)
	c.Assert(err, gc.ErrorMatches, "unauthorized")
}

func (s *httpProxySuite) TestCharmHTTPServe(c *gc.C) {
	ctx := context.Background()
	httpProxier := jujuapi.HTTPProxier(s.JIMM)
	expectU, expectP, err := s.JIMM.GetCredentialStore().GetControllerCredentials(ctx, s.model.Controller.Name)
	c.Assert(err, gc.IsNil)
	// we expect the controller to respond with TLS
	fakeController := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, p, _ := r.BasicAuth()
		c.Assert(u, gc.Equals, names.NewUserTag(expectU).String())
		c.Assert(p, gc.Equals, expectP)
		_, err = w.Write([]byte("OK"))
		c.Assert(err, gc.IsNil)
	}))
	defer fakeController.Close()
	controller := s.model.Controller
	pemData := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: fakeController.Certificate().Raw,
	})
	controller.CACertificate = string(pemData)

	tests := []struct {
		description    string
		setup          func()
		url            string
		statusExpected int
	}{
		{
			description: "good",
			setup: func() {
				newURL, _ := url.Parse(fakeController.URL)
				controller.PublicAddress = newURL.Host
				err = s.JIMM.Database.UpdateController(ctx, &controller)
				c.Assert(err, gc.IsNil)
			},
			url:            fmt.Sprintf("/model/%s/charms", s.model.UUID.String),
			statusExpected: http.StatusOK,
		},
		{
			description: "model not existing",
			setup: func() {
			},
			url:            fmt.Sprintf("/model/%s/charms", "54d9f921-c45a-4825-8253-74e7edc28066"),
			statusExpected: http.StatusNotFound,
		},
	}

	for _, test := range tests {
		test.setup()
		req, err := http.NewRequest("POST", test.url, nil)
		c.Assert(err, gc.IsNil)
		recorder := httptest.NewRecorder()
		httpProxier.ServeHTTP(ctx, recorder, req)
		resp := recorder.Result()
		defer resp.Body.Close()
		c.Assert(resp.StatusCode, gc.Equals, test.statusExpected)
	}
}

type e2eProxySuite struct {
	websocketSuite
}

var _ = gc.Suite(&e2eProxySuite{})

func (s *e2eProxySuite) TestLocalCharmDeploy(c *gc.C) {
	conn := s.open(c, &api.Info{
		ModelTag:  s.Model.ResourceTag(),
		SkipLogin: false,
	}, s.AdminUser.Name)

	client, err := charms.NewLocalCharmClient(conn)
	c.Assert(err, gc.IsNil)
	charmArchive := testcharms.Repo.CharmArchive(c.MkDir(), "dummy")
	curl := charm.MustParseURL(
		fmt.Sprintf("local:quantal/%s-%d", charmArchive.Meta().Name, charmArchive.Revision()),
	)
	vers := version.MustParse("2.6.6")
	url, err := client.AddLocalCharm(curl, charmArchive, false, vers)
	c.Assert(err, gc.IsNil)
	c.Assert(url.String(), gc.Equals, curl.String())

}

func (s *e2eProxySuite) TestResourceEndpoint(c *gc.C) {
	// setup: to upload resource we first need to create the application and the pending resource
	modelState, err := s.StatePool.Get(s.Model.UUID.String)
	c.Assert(err, gc.Equals, nil)
	defer modelState.Release()
	f := factory.NewFactory(modelState.State, s.StatePool)
	charmArchive := testcharms.Repo.CharmArchive(c.MkDir(), "dummy")
	curl := charm.MustParseURL(
		fmt.Sprintf("local:quantal/%s-%d", charmArchive.Meta().Name, charmArchive.Revision()),
	)
	app := f.MakeApplication(c, &factory.ApplicationParams{
		Name: "test-app",
		Charm: f.MakeCharm(c, &factory.CharmParams{
			Name: charmArchive.Meta().Name,
			URL:  curl.String(),
		}),
	})
	pendingId, err := modelState.Resources().AddPendingResource(app.Name(), s.Model.OwnerIdentityName, resource.Resource{
		Meta:   resource.Meta{Name: "test", Type: 1, Path: "file"},
		Origin: resource.OriginStore,
	})
	c.Assert(err, gc.Equals, nil)
	conn := s.open(c, &api.Info{
		ModelTag:  s.Model.ResourceTag(),
		SkipLogin: false,
	}, s.AdminUser.Name)
	uploadClient, err := resources.NewClient(conn)
	c.Assert(err, gc.IsNil)

	// test
	err = uploadClient.Upload(app.Name(), "test", "file", pendingId, strings.NewReader("<data>"))
	c.Assert(err, gc.IsNil)
}
