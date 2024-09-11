// Copyright 2024 Canonical.

package middleware_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	qt "github.com/frankban/quicktest"

	"github.com/canonical/jimm/v3/internal/dbmodel"
	jimm_errors "github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/jimmtest"
	"github.com/canonical/jimm/v3/internal/jimmtest/mocks"
	"github.com/canonical/jimm/v3/internal/middleware"
	"github.com/canonical/jimm/v3/internal/openfga"
)

func AuthorizeUserForModelAccess(t *testing.T) {
	testUser := "test-user@canonical.com"
	jt := jimmtest.JIMM{
		LoginService: mocks.LoginService{
			LoginWithSessionToken_: func(ctx context.Context, sessionToken string) (*openfga.User, error) {
				if sessionToken != "good" {
					return nil, jimm_errors.E(jimm_errors.CodeSessionTokenInvalid)
				}
				user := dbmodel.Identity{Name: testUser}
				return &openfga.User{Identity: &user, JimmAdmin: true}, nil
			},
		},
	}
	tests := []struct {
		name              string
		jimmAdmin         bool
		expectedStatus    int
		basicAuthPassword string
		errorExpected     string
	}{
		{
			name:              "success",
			jimmAdmin:         true,
			expectedStatus:    http.StatusOK,
			basicAuthPassword: "good",
		},
		{
			name:              "failure",
			expectedStatus:    http.StatusUnauthorized,
			basicAuthPassword: "bad",
			errorExpected:     "error authenticating the user",
		},
		{
			name:           "no basic auth",
			expectedStatus: http.StatusUnauthorized,
			errorExpected:  "authentication missing",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := qt.New(t)
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			w := httptest.NewRecorder()
			if tt.basicAuthPassword != "" {
				req.SetBasicAuth("", tt.basicAuthPassword)
			}
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				identity := r.Context().Value(middleware.UserContext{})
				user, ok := identity.(*openfga.User)
				c.Assert(ok, qt.IsTrue)
				c.Assert(user.Name, qt.Equals, testUser)
				w.WriteHeader(http.StatusOK)
			})
			middleware := middleware.AuthenticateViaBasicAuth(handler, &jt)
			middleware.ServeHTTP(w, req)
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
