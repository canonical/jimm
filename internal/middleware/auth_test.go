// Copyright 2024 Canonical.

package middleware_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	rebac_handlers "github.com/canonical/rebac-admin-ui-handlers/v1"
	qt "github.com/frankban/quicktest"

	"github.com/canonical/jimm/v3/internal/auth"
	"github.com/canonical/jimm/v3/internal/dbmodel"
	jimm_errors "github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/jimmtest"
	"github.com/canonical/jimm/v3/internal/jimmtest/mocks"
	"github.com/canonical/jimm/v3/internal/middleware"
	"github.com/canonical/jimm/v3/internal/openfga"
)

// Checks if the authenticator responsible for access control to rebac admin handlers works correctly.
func TestAuthenticateRebac(t *testing.T) {
	testUser := "test-user@canonical.com"
	tests := []struct {
		name                   string
		mockAuthBrowserSession func(ctx context.Context, w http.ResponseWriter, req *http.Request) (context.Context, error)
		jimmAdmin              bool
		expectedStatus         int
	}{
		{
			name: "success",
			mockAuthBrowserSession: func(ctx context.Context, w http.ResponseWriter, req *http.Request) (context.Context, error) {
				return auth.ContextWithSessionIdentity(ctx, testUser), nil
			},
			jimmAdmin:      true,
			expectedStatus: http.StatusOK,
		},
		{
			name: "failure",
			mockAuthBrowserSession: func(ctx context.Context, w http.ResponseWriter, req *http.Request) (context.Context, error) {
				return ctx, errors.New("some error")
			},
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name: "no identity",
			mockAuthBrowserSession: func(ctx context.Context, w http.ResponseWriter, req *http.Request) (context.Context, error) {
				return ctx, nil
			},
			expectedStatus: http.StatusInternalServerError,
		},
		{
			name: "not a jimm admin",
			mockAuthBrowserSession: func(ctx context.Context, w http.ResponseWriter, req *http.Request) (context.Context, error) {
				return auth.ContextWithSessionIdentity(ctx, testUser), nil
			},
			jimmAdmin:      false,
			expectedStatus: http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := qt.New(t)

			j := jimmtest.JIMM{
				LoginService: mocks.LoginService{
					AuthenticateBrowserSession_: func(ctx context.Context, w http.ResponseWriter, req *http.Request) (context.Context, error) {
						return tt.mockAuthBrowserSession(ctx, w, req)
					},
				},
				UserLogin_: func(ctx context.Context, username string) (*openfga.User, error) {
					user := dbmodel.Identity{Name: username}
					return &openfga.User{Identity: &user, JimmAdmin: tt.jimmAdmin}, nil
				},
			}
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			w := httptest.NewRecorder()

			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				identity, err := rebac_handlers.GetIdentityFromContext(r.Context())
				c.Assert(err, qt.IsNil)

				user, ok := identity.(*openfga.User)
				c.Assert(ok, qt.IsTrue)
				c.Assert(user.Name, qt.Equals, testUser)

				w.WriteHeader(http.StatusOK)
			})
			middleware := middleware.AuthenticateRebac(handler, &j)
			middleware.ServeHTTP(w, req)

			c.Assert(w.Code, qt.Equals, tt.expectedStatus)
		})
	}
}

func TestAuthenticateViaBasicAuth(t *testing.T) {
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
