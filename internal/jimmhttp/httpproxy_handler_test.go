// Copyright 2024 Canonical.

package jimmhttp_test

import (
	"context"
	"database/sql"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"

	"github.com/go-chi/chi/v5"
	"github.com/juju/names/v5"
	gc "gopkg.in/check.v1"

	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/jimmhttp"
	"github.com/canonical/jimm/v3/internal/testutils/jimmtest"
)

type httpProxySuite struct {
	jimmtest.JIMMSuite
	model *dbmodel.Model
}

var _ = gc.Suite(&httpProxySuite{})

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

func (s *httpProxySuite) SetUpTest(c *gc.C) {
	s.JIMMSuite.SetUpTest(c)
	ctx := context.Background()
	tester := jimmtest.GocheckTester{C: c}
	env := jimmtest.ParseEnvironment(tester, testEnv)
	env.PopulateDB(tester, s.JIMM.Database)
	model := &dbmodel.Model{UUID: sql.NullString{String: env.Models[0].UUID, Valid: true}}
	err := s.JIMM.Database.GetModel(ctx, model)
	c.Assert(err, gc.IsNil)
	s.model = model
	err = s.JIMM.GetCredentialStore().PutControllerCredentials(ctx, model.Controller.Name, "user", "psw")
	c.Assert(err, gc.IsNil)
}

func (s *httpProxySuite) TestHTTPProxyHandler(c *gc.C) {
	ctx := context.Background()
	httpProxier := jimmhttp.NewHTTPProxyHandler(s.JIMM)
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
		modelUUID      string
		statusExpected int
		bodyExpected   string
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
			modelUUID:      s.model.UUID.String,
			statusExpected: http.StatusOK,
			bodyExpected:   "OK",
		},
		{
			description: "model not existing",
			setup: func() {
			},
			url:            fmt.Sprintf("/model/%s/charms", "54d9f921-c45a-4825-8253-74e7edc28066"),
			modelUUID:      "54d9f921-c45a-4825-8253-74e7edc28066",
			statusExpected: http.StatusNotFound,
			bodyExpected:   ".*failed to get model.*",
		},
	}

	for _, test := range tests {
		if test.setup != nil {
			test.setup()
		}
		req, err := http.NewRequest("POST", test.url, nil)
		c.Assert(err, gc.IsNil)
		recorder := httptest.NewRecorder()
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("uuid", test.modelUUID)
		ctx := context.WithValue(req.Context(), chi.RouteCtxKey, rctx)
		httpProxier.ProxyHTTP(recorder, req.WithContext(ctx))
		resp := recorder.Result()
		defer resp.Body.Close()
		c.Assert(resp.StatusCode, gc.Equals, test.statusExpected)
		body, err := io.ReadAll(resp.Body)
		c.Assert(err, gc.IsNil)
		c.Assert(string(body), gc.Matches, test.bodyExpected)
	}
}
