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
	"github.com/canonical/jimm/v3/internal/middleware"
	"github.com/canonical/jimm/v3/internal/openfga"
	"github.com/canonical/jimm/v3/internal/testutils/jimmtest"
	"github.com/canonical/jimm/v3/internal/testutils/jimmtest/mocks"
)

// Checks if the authenticator responsible for access control to rebac admin handlers works correctly.
func TestAuthenticateRebac(t *testing.T) {
	c := qt.New(t)
	baseURL := "/rebac"

	testUser := "test-user@canonical.com"
	tests := []struct {
		name                   string
		setupRequest           func() *http.Request
		setupHandler           func() http.Handler
		mockAuthBrowserSession func(ctx context.Context, w http.ResponseWriter, req *http.Request) (context.Context, error)
		jimmAdmin              bool
		expectedStatus         int
		expectedBody           string
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
		{
			name: "should skip auth for /swagger.json",
			setupRequest: func() *http.Request {
				req, _ := http.NewRequest(http.MethodGet, baseURL+"/v1/swagger.json", nil)
				return req
			},
			setupHandler: func() http.Handler {
				return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write([]byte("the-body"))
				})
			},
			jimmAdmin:      false,
			expectedStatus: http.StatusOK,
			expectedBody:   "the-body",
		},
	}

	for _, tt := range tests {
		c.Run(tt.name, func(c *qt.C) {
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

			var req *http.Request
			if tt.setupRequest != nil {
				req = tt.setupRequest()
			} else {
				req = httptest.NewRequest(http.MethodGet, baseURL, nil)
			}

			w := httptest.NewRecorder()

			var handler http.Handler
			if tt.setupHandler != nil {
				handler = tt.setupHandler()
			} else {
				handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					identity, err := rebac_handlers.GetIdentityFromContext(r.Context())
					c.Assert(err, qt.IsNil)

					user, ok := identity.(*openfga.User)
					c.Assert(ok, qt.IsTrue)
					c.Assert(user.Name, qt.Equals, testUser)

					w.WriteHeader(http.StatusOK)
				})
			}

			middleware := middleware.AuthenticateRebac(baseURL, handler, &j)
			middleware.ServeHTTP(w, req)

			c.Assert(w.Code, qt.Equals, tt.expectedStatus)
			if tt.expectedBody != "" {
				body, err := io.ReadAll(w.Body)
				c.Assert(err, qt.IsNil)
				c.Assert(string(body), qt.Equals, tt.expectedBody)
			}
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
				user, err := middleware.IdentityFromContext(r.Context())
				c.Assert(err, qt.IsNil)
				c.Assert(user.Name, qt.Equals, testUser)
				w.WriteHeader(http.StatusOK)
			})
			middleware := middleware.AuthenticateWithSessionTokenViaBasicAuth(handler, &jt)
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
