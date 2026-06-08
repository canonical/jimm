// Copyright 2025 Canonical.

package jimmhttp_test

import (
	"context"
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	qt "github.com/frankban/quicktest"
	"github.com/juju/names/v5"

	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/jimm/juju"
	"github.com/canonical/jimm/v3/internal/jimmhttp"
	"github.com/canonical/jimm/v3/internal/middleware"
	"github.com/canonical/jimm/v3/internal/openfga"
	"github.com/canonical/jimm/v3/internal/testutils/jimmtest/mocks"
)

const (
	incomingModelUUID       = "00000001-0000-0000-0000-000000000001"
	migrationControllerUUID = "00000001-0000-0000-0000-000000000001"
)

func TestMigrationHTTPProxyHandler(t *testing.T) {
	c := qt.New(t)
	user := openfga.NewUser(&dbmodel.Identity{Name: "admin@canonical.com"}, nil)
	user.JimmAdmin = true
	modelTag := names.NewModelTag(incomingModelUUID)
	controllerTag := names.NewControllerTag(migrationControllerUUID)
	var gotModelTag names.ModelTag
	var gotControllerTag names.ControllerTag
	var gotUser *openfga.User
	callCount := 0

	fakeController := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c.Check(r.Header.Get("Authorization"), qt.Equals, "Bearer "+base64.StdEncoding.EncodeToString([]byte("test-token")))
		_, err := w.Write([]byte("OK"))
		c.Check(err, qt.IsNil)
	}))
	defer fakeController.Close()

	ctrlService := mocks.ControllerService{
		ControllerDetailsForIncomingModel_: func(ctx context.Context, modelUUID string) (juju.ControllerConnectionDetails, error) {
			if modelUUID != incomingModelUUID {
				return juju.ControllerConnectionDetails{}, errors.Codef(errors.CodeNotFound, "model not found")
			}
			return juju.ControllerConnectionDetails{
				ControllerUUID: migrationControllerUUID,
				PublicAddress:  fakeController.URL,
			}, nil
		},
	}
	loginTokens := loginTokenProvider{NewSuperuserLoginToken_: func(ctx context.Context, gotMT names.ModelTag, gotCT names.ControllerTag, gotU *openfga.User) ([]byte, error) {
		callCount++
		gotModelTag = gotMT
		gotControllerTag = gotCT
		gotUser = gotU
		return []byte("test-token"), nil
	}}
	migrationProxier := jimmhttp.NewMigrationHTTPProxyHandler(nil, &ctrlService, loginTokens)

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
			headers:        http.Header{"X-Juju-Migration-Model-UUID": []string{incomingModelUUID}},
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

	c.Assert(callCount, qt.Equals, 1)
	c.Assert(gotModelTag, qt.Equals, modelTag)
	c.Assert(gotControllerTag, qt.Equals, controllerTag)
	c.Assert(gotUser, qt.Equals, user)
}
