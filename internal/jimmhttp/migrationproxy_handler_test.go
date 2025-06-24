// Copyright 2025 Canonical.

package jimmhttp_test

import (
	"context"
	"database/sql"
	"encoding/pem"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"

	"github.com/juju/names/v5"
	gc "gopkg.in/check.v1"

	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/jimmhttp"
	"github.com/canonical/jimm/v3/internal/testutils/jimmtest"
)

type migrationHTTPProxySuite struct {
	jimmtest.JIMMSuite
	incomingModel *dbmodel.IncomingModelMigration
}

var _ = gc.Suite(&migrationHTTPProxySuite{})

const (
	incomingModelUUID = "00000001-0000-0000-0000-000000000001"

	migrationTestEnv = `
clouds:
- name: test-cloud
  type: test-provider
  regions:
  - name: test-cloud-region
controllers:
- name: controller-1
  uuid: 00000001-0000-0000-0000-000000000001
  cloud: test-cloud
  region: test-cloud-region
`
)

func (s *migrationHTTPProxySuite) SetUpTest(c *gc.C) {
	s.JIMMSuite.SetUpTest(c)
	ctx := context.Background()
	tester := jimmtest.GocheckTester{C: c}
	env := jimmtest.ParseEnvironment(tester, migrationTestEnv)
	env.PopulateDB(tester, s.JIMM.Database)

	incomingModel := &dbmodel.IncomingModelMigration{
		TargetControllerID: env.Controllers[0].DBObject(tester, s.JIMM.Database).ID,
		ModelUUID:          sql.NullString{String: incomingModelUUID, Valid: true},
		UserMapping:        map[string]string{"bob": "alice@canonical.com"},
	}
	err := s.JIMM.Database.AddIncomingModelMigration(ctx, incomingModel)
	c.Assert(err, gc.IsNil)
	s.incomingModel = incomingModel

	err = s.JIMM.CredentialStore.PutControllerCredentials(ctx, env.Controllers[0].Name, "user", "psw")
	c.Assert(err, gc.IsNil)
}

func (s *migrationHTTPProxySuite) TestMigrationHTTPProxyHandler(c *gc.C) {
	ctx := context.Background()

	controllerName := "controller-1"
	migrationProxier := jimmhttp.NewMigrationHTTPProxyHandler(s.JIMM)
	expectUsername, expectPassword, err := s.JIMM.CredentialStore.GetControllerCredentials(ctx, controllerName)
	c.Assert(err, gc.IsNil)

	// we expect the controller to respond with TLS
	fakeController := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, p, _ := r.BasicAuth()
		c.Check(u, gc.Equals, names.NewUserTag(expectUsername).String())
		c.Check(p, gc.Equals, expectPassword)
		_, err = w.Write([]byte("OK"))
		c.Check(err, gc.IsNil)
	}))
	defer fakeController.Close()

	// Change a controller in the database to have the address of our fake server.
	controller := dbmodel.Controller{
		Name: controllerName,
	}
	err = s.JIMM.Database.GetController(ctx, &controller)
	c.Assert(err, gc.IsNil)

	pemData := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: fakeController.Certificate().Raw,
	})
	controller.CACertificate = string(pemData)
	newURL, _ := url.Parse(fakeController.URL)
	controller.PublicAddress = newURL.Host

	err = s.JIMM.Database.UpdateController(ctx, &controller)
	c.Assert(err, gc.IsNil)

	tests := []struct {
		description    string
		headers        http.Header
		url            string
		statusExpected int
		bodyExpected   string
	}{
		{
			description:    "success",
			url:            "/foo",
			headers:        http.Header{"X-Juju-Migration-Model-UUID": []string{s.incomingModel.ModelUUID.String}},
			statusExpected: http.StatusOK,
			bodyExpected:   "OK",
		},
		{
			description:    "model isn't migrating",
			url:            "/foo",
			headers:        http.Header{"X-Juju-Migration-Model-UUID": []string{"non-existent-model-uuid"}},
			statusExpected: http.StatusNotFound,
			bodyExpected:   `Not Found - migrating model "non-existent-model-uuid" not found`,
		},
		{
			description:    "missing model header",
			url:            "/foo",
			headers:        http.Header{"X-Juju-Migration-Model-UUID": []string{""}},
			statusExpected: http.StatusBadRequest,
			bodyExpected:   "Bad Request - missing X-Juju-Migration-Model-UUID header value",
		},
	}

	for _, test := range tests {
		req, err := http.NewRequest("POST", test.url, nil)
		c.Assert(err, gc.IsNil)
		for key, values := range test.headers {
			for _, value := range values {
				req.Header.Add(key, value)
			}
		}

		recorder := httptest.NewRecorder()
		migrationProxier.ProxyHTTP(recorder, req)

		resp := recorder.Result()
		defer resp.Body.Close()
		c.Assert(resp.StatusCode, gc.Equals, test.statusExpected)

		body, err := io.ReadAll(resp.Body)
		c.Assert(err, gc.IsNil)
		c.Assert(string(body), gc.Matches, test.bodyExpected)
	}
}
