// Copyright 2025 Canonical.

package jimmhttp_test

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	qt "github.com/frankban/quicktest"
	"github.com/go-chi/chi/v5"
	"github.com/juju/names/v5"

	"github.com/canonical/jimm/v3/internal/db"
	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/jimm/juju"
	"github.com/canonical/jimm/v3/internal/jimmhttp"
	"github.com/canonical/jimm/v3/internal/testutils/jimmtest"
	"github.com/canonical/jimm/v3/internal/testutils/jimmtest/mocks"
	"github.com/canonical/jimm/v3/internal/testutils/testdb"
)

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

func TestHTTPProxyHandler(t *testing.T) {
	c := qt.New(t)
	db := &db.Database{
		DB: testdb.PostgresDB(c, time.Now),
	}
	err := db.Migrate(context.Background())
	c.Assert(err, qt.IsNil)

	env := jimmtest.ParseEnvironment(c, testEnv)
	env.PopulateDB(c, db)
	model := &dbmodel.Model{UUID: sql.NullString{String: env.Models[0].UUID, Valid: true}}
	err = db.GetModel(c.Context(), model)
	c.Assert(err, qt.IsNil)

	fakeController := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, p, _ := r.BasicAuth()
		c.Check(u, qt.Equals, names.NewUserTag("admin").String())
		c.Check(p, qt.Equals, "test")
		_, err := w.Write([]byte("OK"))
		c.Check(err, qt.IsNil)
	}))
	defer fakeController.Close()

	ctrlService := mocks.ControllerService{
		ControllerDetailsForModel_: func(ctx context.Context, modelUUID string) (juju.ControllerConnectionDetails, error) {
			if modelUUID != model.UUID.String {
				return juju.ControllerConnectionDetails{}, errors.Codef(errors.CodeNotFound, "model not found")
			}
			return juju.ControllerConnectionDetails{
				PublicAddress: fakeController.URL,
				Credentials: juju.ControllerCreds{
					AdminPassword:     "test",
					AdminIdentityName: "admin",
				},
			}, nil
		}}
	httpProxier := jimmhttp.NewHTTPProxyHandler(nil, &ctrlService)

	tests := []struct {
		description    string
		url            string
		modelUUID      string
		statusExpected int
		bodyExpected   string
	}{
		{
			description:    "good",
			url:            fmt.Sprintf("/model/%s/charms", model.UUID.String),
			modelUUID:      model.UUID.String,
			statusExpected: http.StatusOK,
			bodyExpected:   "OK",
		},
		{
			description:    "invalid model UUID",
			url:            fmt.Sprintf("/model/%s/charms", "fake-uuid"),
			modelUUID:      "fake-uuid",
			statusExpected: http.StatusBadRequest,
			bodyExpected:   "Bad Request - invalid model UUID format\n",
		},
		{
			description:    "model not existing",
			url:            fmt.Sprintf("/model/%s/charms", "54d9f921-c45a-4825-8253-74e7edc28066"),
			modelUUID:      "54d9f921-c45a-4825-8253-74e7edc28066",
			statusExpected: http.StatusNotFound,
			bodyExpected:   "Not Found - model not found\n",
		},
	}

	for _, test := range tests {
		c.Run(test.description, func(c *qt.C) {
			req, err := http.NewRequest("POST", test.url, nil)
			c.Assert(err, qt.IsNil)

			recorder := httptest.NewRecorder()
			rctx := chi.NewRouteContext()
			rctx.URLParams.Add("uuid", test.modelUUID)
			ctx := context.WithValue(req.Context(), chi.RouteCtxKey, rctx)

			httpProxier.ProxyHTTP(recorder, req.WithContext(ctx))
			resp := recorder.Result()
			defer resp.Body.Close()

			c.Assert(resp.StatusCode, qt.Equals, test.statusExpected)
			body, err := io.ReadAll(resp.Body)
			c.Assert(err, qt.IsNil)
			c.Assert(string(body), qt.Matches, test.bodyExpected)
		})
	}
}
