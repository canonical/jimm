// Copyright 2024 Canonical.

package middleware_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	cofga "github.com/canonical/ofga"
	qt "github.com/frankban/quicktest"
	"github.com/go-chi/chi/v5"
	"github.com/juju/names/v5"

	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/middleware"
	"github.com/canonical/jimm/v3/internal/openfga"
	ofganames "github.com/canonical/jimm/v3/internal/openfga/names"
	"github.com/canonical/jimm/v3/internal/testutils/jimmtest"
)

func TestAuthorizeUserForModelAccess(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()
	ofgaClient, _, _, err := jimmtest.SetupTestOFGAClient(t.Name())
	c.Assert(err, qt.IsNil)
	bobIdentity, err := dbmodel.NewIdentity("bob@canonical.com")
	c.Assert(err, qt.IsNil)
	bob := openfga.NewUser(bobIdentity, ofgaClient)
	validModelUUID := "54d9f921-c45a-4825-8253-74e7edc28066"
	notvalidModelUUID := "54d9f921-c45a-4825-8253-74e7edc28065"
	tuples := []openfga.Tuple{
		{
			Object:   ofganames.ConvertTag(bob.ResourceTag()),
			Relation: ofganames.AdministratorRelation,
			Target:   ofganames.ConvertTag(names.NewModelTag(validModelUUID)),
		},
		{
			Object:   ofganames.ConvertTag(bob.ResourceTag()),
			Relation: ofganames.ReaderRelation,
			Target:   ofganames.ConvertTag(names.NewModelTag(notvalidModelUUID)),
		},
	}
	err = ofgaClient.AddRelation(ctx, tuples...)
	c.Assert(err, qt.IsNil)
	tests := []struct {
		name               string
		expectedStatus     int
		uuidInPath         string
		permissionRequired cofga.Relation
		errorExpected      string
	}{
		{
			name:               "success",
			expectedStatus:     http.StatusOK,
			permissionRequired: ofganames.WriterRelation,
			uuidInPath:         validModelUUID,
		},
		{
			name:           "no uuid from path",
			expectedStatus: http.StatusUnauthorized,
			errorExpected:  "cannot find uuid in URL path",
		},
		{
			name:               "not enough permission",
			expectedStatus:     http.StatusForbidden,
			uuidInPath:         notvalidModelUUID,
			permissionRequired: ofganames.WriterRelation,
			errorExpected:      "no access to the resource",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := qt.New(t)
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			w := httptest.NewRecorder()
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			})
			h := middleware.AuthorizeUserForModelAccess(handler, tt.permissionRequired)
			rctx := chi.NewRouteContext()
			rctx.URLParams.Add("uuid", tt.uuidInPath)
			ctx := context.WithValue(req.Context(), chi.RouteCtxKey, rctx)
			h.ServeHTTP(w, req.WithContext(middleware.WithIdentity(ctx, bob)))
			c.Assert(w.Code, qt.Equals, tt.expectedStatus)
			b := w.Result().Body
			defer b.Close()
			body, err := io.ReadAll(b)
			c.Assert(err, qt.IsNil)
			if tt.errorExpected != "" {
				c.Assert(string(body), qt.Matches, tt.errorExpected)
			}
		})
	}
}
