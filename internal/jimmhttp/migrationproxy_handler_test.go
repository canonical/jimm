// Copyright 2025 Canonical.

package jimmhttp_test

import (
	"context"
	"database/sql"
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	qt "github.com/frankban/quicktest"
	"github.com/juju/names/v5"

	"github.com/canonical/jimm/v3/internal/db"
	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/jimm/juju"
	"github.com/canonical/jimm/v3/internal/jimmhttp"
	"github.com/canonical/jimm/v3/internal/jimmjwx"
	"github.com/canonical/jimm/v3/internal/middleware"
	"github.com/canonical/jimm/v3/internal/openfga"
	"github.com/canonical/jimm/v3/internal/testutils/jimmtest"
	"github.com/canonical/jimm/v3/internal/testutils/jimmtest/mocks"
	"github.com/canonical/jimm/v3/internal/testutils/testdb"
)

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

func TestMigrationHTTPProxyHandler(t *testing.T) {
	c := qt.New(t)
	db := &db.Database{
		DB: testdb.PostgresDB(c, time.Now),
	}
	err := db.Migrate(context.Background())
	c.Assert(err, qt.IsNil)

	env := jimmtest.ParseEnvironment(c, migrationTestEnv)
	env.PopulateDB(c, db)

	incomingModel := &dbmodel.IncomingModelMigration{
		TargetControllerID: env.Controllers[0].DBObject(c, db).ID,
		ModelUUID:          sql.NullString{String: incomingModelUUID, Valid: true},
		UserMapping:        map[string]string{"bob": "alice@canonical.com"},
	}
	err = db.AddOrUpdateIncomingModelMigration(c.Context(), incomingModel)
	c.Assert(err, qt.IsNil)
	user := openfga.NewUser(&dbmodel.Identity{Name: "admin@canonical.com"}, nil)
	user.JimmAdmin = true
	var gotJWTParams jimmjwx.JWTParams

	fakeController := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c.Check(r.Header.Get("Authorization"), qt.Equals, "Bearer "+base64.StdEncoding.EncodeToString([]byte("test-token")))
		_, err := w.Write([]byte("OK"))
		c.Check(err, qt.IsNil)
	}))
	defer fakeController.Close()

	ctrlService := mocks.ControllerService{
		ControllerDetailsForIncomingModel_: func(ctx context.Context, modelUUID string) (juju.ControllerConnectionDetails, error) {
			if modelUUID != incomingModel.ModelUUID.String {
				return juju.ControllerConnectionDetails{}, errors.Codef(errors.CodeNotFound, "model not found")
			}
			return juju.ControllerConnectionDetails{
				ControllerUUID: env.Controllers[0].UUID,
				PublicAddress:  fakeController.URL,
			}, nil
		}}
	jwtService := mocks.JWTService{NewJWT_: func(ctx context.Context, params jimmjwx.JWTParams) ([]byte, error) {
		gotJWTParams = params
		return []byte("test-token"), nil
	}}
	migrationProxier := jimmhttp.NewMigrationHTTPProxyHandler(nil, &ctrlService, jwtService)

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
			headers:        http.Header{"X-Juju-Migration-Model-UUID": []string{incomingModel.ModelUUID.String}},
			statusExpected: http.StatusOK,
			bodyExpected:   "OK",
		},
		{
			description:    "model isn't migrating",
			url:            "/foo",
			headers:        http.Header{"X-Juju-Migration-Model-UUID": []string{"non-existent-model-uuid"}},
			statusExpected: http.StatusNotFound,
			bodyExpected:   `Not Found - model not found\n`,
		},
		{
			description:    "missing model header",
			url:            "/foo",
			headers:        http.Header{"X-Juju-Migration-Model-UUID": []string{""}},
			statusExpected: http.StatusBadRequest,
			bodyExpected:   "Bad Request - missing X-Juju-Migration-Model-UUID header value\n",
		},
	}

	for _, test := range tests {
		c.Run(test.description, func(c *qt.C) {
			req, err := http.NewRequest("POST", test.url, nil)
			c.Assert(err, qt.IsNil)
			for key, values := range test.headers {
				for _, value := range values {
					req.Header.Add(key, value)
				}
			}
			req = req.WithContext(middleware.ContextWithIdentity(req.Context(), user))

			recorder := httptest.NewRecorder()
			migrationProxier.ProxyHTTP(recorder, req)

			resp := recorder.Result()
			defer resp.Body.Close()
			c.Assert(resp.StatusCode, qt.Equals, test.statusExpected)

			body, err := io.ReadAll(resp.Body)
			c.Assert(err, qt.IsNil)
			c.Assert(string(body), qt.Matches, test.bodyExpected)
		})
	}

	c.Assert(gotJWTParams, qt.DeepEquals, jimmjwx.JWTParams{
		Controller: env.Controllers[0].UUID,
		User:       names.NewUserTag("admin@canonical.com").String(),
		Access: map[string]string{
			names.NewControllerTag(env.Controllers[0].UUID).String(): "superuser",
		},
	})
}
