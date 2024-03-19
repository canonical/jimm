// Copyright 2023 canonical.

package credentials

import (
	"context"
	"time"

	"github.com/juju/names/v5"
	"github.com/lestrrat-go/jwx/v2/jwk"
	"github.com/stretchr/testify/mock"
)

// MockCredentialStore is a mock implementation of the `CredentialStore` interface.
type MockCredentialStore struct {
	mock.Mock
}

func (m *MockCredentialStore) Get(ctx context.Context, credTag names.CloudCredentialTag) (map[string]string, error) {
	args := m.Called(ctx, credTag)
	return args.Get(0).(map[string]string), args.Error(1)
}

func (m *MockCredentialStore) Put(ctx context.Context, cloudCredTag names.CloudCredentialTag, attrs map[string]string) error {
	args := m.Called(ctx, cloudCredTag, attrs)
	return args.Error(0)
}

func (m *MockCredentialStore) GetControllerCredentials(ctx context.Context, controllerName string) (string, string, error) {
	args := m.Called(ctx, controllerName)
	return args.String(0), args.String(1), args.Error(2)
}

func (m *MockCredentialStore) PutControllerCredentials(ctx context.Context, controllerName string, username string, password string) error {
	args := m.Called(ctx, controllerName, username, password)
	return args.Error(0)
}

func (m *MockCredentialStore) CleanupJWKS(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

func (m *MockCredentialStore) GetJWKS(ctx context.Context) (jwk.Set, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(jwk.Set), args.Error(1)
}

func (m *MockCredentialStore) GetJWKSPrivateKey(ctx context.Context) ([]byte, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]byte), args.Error(1)
}

func (m *MockCredentialStore) GetJWKSExpiry(ctx context.Context) (time.Time, error) {
	args := m.Called(ctx)
	return args.Get(0).(time.Time), args.Error(1)
}

func (m *MockCredentialStore) PutJWKS(ctx context.Context, jwks jwk.Set) error {
	args := m.Called(ctx, jwks)
	return args.Error(0)
}

func (m *MockCredentialStore) PutJWKSPrivateKey(ctx context.Context, pem []byte) error {
	args := m.Called(ctx, pem)
	return args.Error(0)
}

func (m *MockCredentialStore) PutJWKSExpiry(ctx context.Context, expiry time.Time) error {
	args := m.Called(ctx, expiry)
	return args.Error(0)
}

func (m *MockCredentialStore) CleanupOAuthSecrets(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

func (m *MockCredentialStore) GetOAuthSecret(ctx context.Context) ([]byte, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]byte), args.Error(1)
}

func (m *MockCredentialStore) PutOAuthSecret(ctx context.Context, raw []byte) error {
	args := m.Called(ctx, raw)
	return args.Error(0)
}
