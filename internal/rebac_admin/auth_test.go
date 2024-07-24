// Copyright 2024 Canonical Ltd.

package rebac_admin_test

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
	"github.com/canonical/jimm/internal/rebac_admin"
)

// Checks if the authenticator responsible for access control to rebac admin handlers works correctly.
func TestAuthenticate(t *testing.T) {
	tests := []struct {
		name          string
		setupMock     func(*jimmtest.MockOAuthAuthenticator)
		expectedError string
	}{
		{
			name: "success",
			setupMock: func(m *jimmtest.MockOAuthAuthenticator) {
				m.AuthenticateBrowserSession_ = func(ctx context.Context, w http.ResponseWriter, req *http.Request) (context.Context, error) {
					return auth.ContextWithSessionIdentity(ctx, "test-user@canonical.com"), nil
				}
			},
		},
		{
			name: "failure",
			setupMock: func(m *jimmtest.MockOAuthAuthenticator) {
				m.AuthenticateBrowserSession_ = func(ctx context.Context, w http.ResponseWriter, req *http.Request) (context.Context, error) {
					return ctx, errors.New("some error")
				}
			},
			expectedError: "failed to authenticate",
		},
		{
			name: "no identity",
			setupMock: func(m *jimmtest.MockOAuthAuthenticator) {
				m.AuthenticateBrowserSession_ = func(ctx context.Context, w http.ResponseWriter, req *http.Request) (context.Context, error) {
					return ctx, nil
				}
			},
			expectedError: "no identity found in session",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := qt.New(t)

			client, _, _, err := jimmtest.SetupTestOFGAClient(c.Name())
			c.Assert(err, qt.IsNil)

			mockAuthService := jimmtest.NewMockOAuthAuthenticator("")
			tt.setupMock(&mockAuthService)

			j := &jimm.JIMM{
				UUID: uuid.NewString(),
				Database: db.Database{
					DB: jimmtest.PostgresDB(c, func() time.Time { return time.Now() }),
				},
				OpenFGAClient:      client,
				OAuthAuthenticator: mockAuthService,
			}
			err = j.Database.Migrate(context.Background(), false)
			c.Assert(err, qt.IsNil)

			authenticator := rebac_admin.NewAuthenticator(j)

			req := httptest.NewRequest(http.MethodGet, "/", nil)
			_, err = authenticator.Authenticate(req)

			if tt.expectedError != "" {
				qt.Assert(t, err, qt.Not(qt.IsNil))
				qt.Assert(t, err.Error(), qt.Contains, tt.expectedError)
			} else {
				qt.Assert(t, err, qt.IsNil)
			}
		})
	}
}
