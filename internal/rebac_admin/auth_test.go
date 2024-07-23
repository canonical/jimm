// Copyright 2024 Canonical Ltd.

package rebac_admin_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"errors"

	"github.com/canonical/jimm"
	"github.com/canonical/jimm/internal/auth"
	"github.com/canonical/jimm/internal/jimmtest"
	"github.com/canonical/jimm/internal/rebac_admin"
	qt "github.com/frankban/quicktest"
)

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
					return ctx, errors.New("")
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

			_, _, cofgaParams, err := jimmtest.SetupTestOFGAClient(c.Name())
			c.Assert(err, qt.IsNil)

			p := jimmtest.NewTestJimmParams(c)
			p.OpenFGAParams = jimmtest.CofgaParamsToJIMMOpenFGAParams(*cofgaParams)
			p.InsecureSecretStorage = true
			svc, err := jimm.NewService(context.Background(), p)
			c.Assert(err, qt.IsNil)
			defer svc.Cleanup()

			mockAuthService := jimmtest.NewMockOAuthAuthenticator("")
			tt.setupMock(&mockAuthService)
			svc.JIMM().OAuthAuthenticator = mockAuthService

			authenticator := rebac_admin.NewAuthenticator(svc.JIMM())

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
