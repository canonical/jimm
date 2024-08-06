// Copyright 2024 Canonical Ltd.

package middleware_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	qt "github.com/frankban/quicktest"

	"github.com/canonical/jimm/v3/internal/auth"
	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/jimm"
	"github.com/canonical/jimm/v3/internal/jimmtest"
	"github.com/canonical/jimm/v3/internal/middleware"
	"github.com/canonical/jimm/v3/internal/openfga"
	rebac_handlers "github.com/canonical/rebac-admin-ui-handlers/v1"
)

// Checks if the authenticator responsible for access control to rebac admin handlers works correctly.
func TestAuthenticateRebac(t *testing.T) {
	testUser := "test-user@canonical.com"
	tests := []struct {
		name           string
		setupMock      func(*jimmtest.MockOAuthAuthenticator)
		jimmAdmin      bool
		expectedStatus int
	}{
		{
			name: "success",
			setupMock: func(m *jimmtest.MockOAuthAuthenticator) {
				m.AuthenticateBrowserSession_ = func(ctx context.Context, w http.ResponseWriter, req *http.Request) (context.Context, error) {
					return auth.ContextWithSessionIdentity(ctx, testUser), nil
				}
			},
			jimmAdmin:      true,
			expectedStatus: http.StatusOK,
		},
		{
			name: "failure",
			setupMock: func(m *jimmtest.MockOAuthAuthenticator) {
				m.AuthenticateBrowserSession_ = func(ctx context.Context, w http.ResponseWriter, req *http.Request) (context.Context, error) {
					return ctx, errors.New("some error")
				}
			},
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name: "no identity",
			setupMock: func(m *jimmtest.MockOAuthAuthenticator) {
				m.AuthenticateBrowserSession_ = func(ctx context.Context, w http.ResponseWriter, req *http.Request) (context.Context, error) {
					return ctx, nil
				}
			},
			expectedStatus: http.StatusInternalServerError,
		},
		{
			name: "not a jimm admin",
			setupMock: func(m *jimmtest.MockOAuthAuthenticator) {
				m.AuthenticateBrowserSession_ = func(ctx context.Context, w http.ResponseWriter, req *http.Request) (context.Context, error) {
					return auth.ContextWithSessionIdentity(ctx, testUser), nil
				}
			},
			jimmAdmin:      false,
			expectedStatus: http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := qt.New(t)

			mockAuthService := jimmtest.NewMockOAuthAuthenticator(nil, nil)
			tt.setupMock(&mockAuthService)

			j := jimmtest.JIMM{
				OAuthAuthenticationService_: func() jimm.OAuthAuthenticator {
					return &mockAuthService
				},
				GetUser_: func(ctx context.Context, username string) (*openfga.User, error) {
					user := dbmodel.Identity{Name: username}
					return &openfga.User{Identity: &user, JimmAdmin: tt.jimmAdmin}, nil
				},
				UpdateUserLastLogin_: func(ctx context.Context, identifier string) error {
					return nil
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
