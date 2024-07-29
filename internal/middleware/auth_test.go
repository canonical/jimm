// Copyright 2024 Canonical Ltd.

package middleware_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	qt "github.com/frankban/quicktest"
	"github.com/google/uuid"

	"github.com/canonical/jimm/internal/auth"
	"github.com/canonical/jimm/internal/db"
	"github.com/canonical/jimm/internal/jimm"
	"github.com/canonical/jimm/internal/jimmtest"
	"github.com/canonical/jimm/internal/middleware"
	"github.com/canonical/jimm/internal/openfga"
	rebac_handlers "github.com/canonical/rebac-admin-ui-handlers/v1"
)

// Checks if the authenticator responsible for access control to rebac admin handlers works correctly.
func TestAuthenticate(t *testing.T) {
	testUser := "test-user@canonical.com"
	tests := []struct {
		name           string
		setupMock      func(*jimmtest.MockOAuthAuthenticator)
		expectedStatus int
	}{
		{
			name: "success",
			setupMock: func(m *jimmtest.MockOAuthAuthenticator) {
				m.AuthenticateBrowserSession_ = func(ctx context.Context, w http.ResponseWriter, req *http.Request) (context.Context, error) {
					return auth.ContextWithSessionIdentity(ctx, testUser), nil
				}
			},
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := qt.New(t)

			client, _, _, err := jimmtest.SetupTestOFGAClient(c.Name())
			c.Assert(err, qt.IsNil)

			mockAuthService := jimmtest.NewMockOAuthAuthenticator(nil, nil)
			tt.setupMock(&mockAuthService)

			j := &jimm.JIMM{
				UUID: uuid.NewString(),
				Database: db.Database{
					DB: jimmtest.PostgresDB(c, func() time.Time { return time.Now() }),
				},
				OpenFGAClient:      client,
				OAuthAuthenticator: &mockAuthService,
			}
			err = j.Database.Migrate(context.Background(), false)
			c.Assert(err, qt.IsNil)

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
			middleware := middleware.AuthenticateRebac(handler, j)
			middleware.ServeHTTP(w, req)

			c.Assert(w.Code, qt.Equals, tt.expectedStatus)
		})
	}
}
