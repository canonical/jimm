// Copyright 2026 Canonical.

package jujuapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	qt "github.com/frankban/quicktest"
	"go.uber.org/mock/gomock"

	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/middleware"
	"github.com/canonical/jimm/v3/internal/openfga"
)

func TestAuthenticateStreamBasicAuth(t *testing.T) {
	newUser := func(name string, groups []string) *openfga.User {
		user := openfga.NewUser(&dbmodel.Identity{Name: name}, nil)
		user.SetIDPGroups(groups)
		return user
	}

	sessionUser := newUser("user@example.com", []string{"engineering"})
	serviceAccountUser := newUser("service@serviceaccount", []string{"platform"})

	tests := []struct {
		name          string
		username      string
		password      string
		setBasicAuth  bool
		setupMock     func(*MockStreamLoginManager)
		expectedUser  *openfga.User
		expectedError string
	}{
		{
			name:         "session token",
			password:     "session-token",
			setBasicAuth: true,
			setupMock: func(mockLoginManager *MockStreamLoginManager) {
				mockLoginManager.EXPECT().LoginWithSessionToken(gomock.Any(), "session-token").Return(sessionUser, nil)
			},
			expectedUser: sessionUser,
		},
		{
			name:         "client credentials",
			username:     "service",
			password:     "client-secret",
			setBasicAuth: true,
			setupMock: func(mockLoginManager *MockStreamLoginManager) {
				mockLoginManager.EXPECT().LoginClientCredentials(gomock.Any(), "service", "client-secret").Return(serviceAccountUser, nil)
			},
			expectedUser: serviceAccountUser,
		},
		{
			name:          "missing authentication",
			expectedError: "authentication missing",
		},
		{
			name:         "invalid client credentials",
			username:     "service",
			password:     "invalid",
			setBasicAuth: true,
			setupMock: func(mockLoginManager *MockStreamLoginManager) {
				mockLoginManager.EXPECT().LoginClientCredentials(gomock.Any(), "service", "invalid").Return(nil, errors.New("invalid client credentials"))
			},
			expectedError: "invalid client credentials",
		},
		{
			name:          "no basic auth header",
			setBasicAuth:  false,
			expectedError: "authentication missing",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			c := qt.New(t)
			ctrl := gomock.NewController(t)
			loginManager := NewMockStreamLoginManager(ctrl)
			if test.setupMock != nil {
				test.setupMock(loginManager)
			}
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if test.setBasicAuth {
				req.SetBasicAuth(test.username, test.password)
			}

			ctx, err := authenticateStreamBasicAuth(context.Background(), req, loginManager)
			if test.expectedError != "" {
				c.Assert(err, qt.ErrorMatches, test.expectedError)
				c.Assert(errors.ErrorCode(err), qt.Equals, errors.CodeUnauthorized)
				return
			}

			c.Assert(err, qt.IsNil)
			user, err := middleware.IdentityFromContext(ctx)
			c.Assert(err, qt.IsNil)
			c.Assert(user, qt.Equals, test.expectedUser)
			c.Assert(user.IDPGroupIDs, qt.DeepEquals, test.expectedUser.IDPGroupIDs)
		})
	}
}
