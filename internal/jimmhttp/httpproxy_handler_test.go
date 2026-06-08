// Copyright 2025 Canonical.

package jimmhttp_test

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	qt "github.com/frankban/quicktest"
	"github.com/go-chi/chi/v5"
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
	httpProxyControllerUUID = "00000001-0000-0000-0000-000000000001"
	httpProxyModelUUID      = "00000002-0000-0000-0000-000000000001"
)

func TestHTTPProxyHandler(t *testing.T) {
	c := qt.New(t)
	user := openfga.NewUser(&dbmodel.Identity{Name: "alice@canonical.com"}, nil)
	modelTag := names.NewModelTag(httpProxyModelUUID)
	controllerTag := names.NewControllerTag(httpProxyControllerUUID)
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
		ControllerDetailsForModel_: func(ctx context.Context, modelUUID string) (juju.ControllerConnectionDetails, error) {
			if modelUUID != httpProxyModelUUID {
				return juju.ControllerConnectionDetails{}, errors.Codef(errors.CodeNotFound, "model not found")
			}
			return juju.ControllerConnectionDetails{
				ControllerUUID: httpProxyControllerUUID,
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
	httpProxier := jimmhttp.NewHTTPProxyHandler(nil, &ctrlService, loginTokens)

	tests := []struct {
		description    string
		url            string
		modelUUID      string
		statusExpected int
		bodyExpected   string
	}{
		{
			description:    "good",
			url:            fmt.Sprintf("/model/%s/charms", httpProxyModelUUID),
			modelUUID:      httpProxyModelUUID,
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
			ctx := middleware.ContextWithIdentity(context.WithValue(req.Context(), chi.RouteCtxKey, rctx), user)

			httpProxier.ProxyHTTP(recorder, req.WithContext(ctx))
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

type loginTokenProvider struct {
	NewSuperuserLoginToken_ func(ctx context.Context, modelTag names.ModelTag, controllerTag names.ControllerTag, user *openfga.User) ([]byte, error)
}

func (p loginTokenProvider) NewSuperuserLoginToken(ctx context.Context, modelTag names.ModelTag, controllerTag names.ControllerTag, user *openfga.User) ([]byte, error) {
	return p.NewSuperuserLoginToken_(ctx, modelTag, controllerTag, user)
}
