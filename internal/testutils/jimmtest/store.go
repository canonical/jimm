// Copyright 2025 Canonical.

package jimmtest

import (
	"context"
	"maps"
	"sync"

	"github.com/juju/names/v5"

	"github.com/canonical/jimm/v3/internal/errors"
)

type controllerCredentials struct {
	username string
	password string
}

// InMemoryCredentialStore implements CredentialStore but only implements
// JWKS methods in order to use it as an in memory credential store replacing
// vault for tests.
type InMemoryCredentialStore struct {
	mu                        sync.RWMutex
	controllerCredentials     map[string]controllerCredentials
	cloudCredentialAttributes map[string]map[string]string
}

// NewInMemoryCredentialStore returns a new instance of `InMemoryCredentialStore`
// with some secrets/keys being populated.
func NewInMemoryCredentialStore() *InMemoryCredentialStore {
	return &InMemoryCredentialStore{}
}

// Get retrieves the stored attributes of a cloud credential.
func (s *InMemoryCredentialStore) Get(ctx context.Context, credTag names.CloudCredentialTag) (map[string]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	attrs, ok := s.cloudCredentialAttributes[credTag.String()]
	if !ok {
		return nil, errors.Codef(errors.CodeNotFound, "not found")
	}
	attrsCopy := make(map[string]string, len(attrs))
	maps.Copy(attrsCopy, attrs)
	return attrsCopy, nil
}

// Put stores the attributes of a cloud credential.
func (s *InMemoryCredentialStore) Put(ctx context.Context, credTag names.CloudCredentialTag, attrs map[string]string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.cloudCredentialAttributes == nil {
		s.cloudCredentialAttributes = make(map[string]map[string]string)
	}

	attrsCopy := make(map[string]string, len(attrs))
	maps.Copy(attrsCopy, attrs)
	s.cloudCredentialAttributes[credTag.String()] = attrsCopy
	return nil
}

// GetControllerCredentials retrieves the credentials for the given controller from a vault
// service.
func (s *InMemoryCredentialStore) GetControllerCredentials(ctx context.Context, controllerName string) (string, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	cc, ok := s.controllerCredentials[controllerName]
	if !ok {
		return "", "", errors.Codef(errors.CodeNotFound, "not found")
	}
	return cc.username, cc.password, nil
}

// PutControllerCredentials stores the controller credentials in a vault
// service.
func (s *InMemoryCredentialStore) PutControllerCredentials(ctx context.Context, controllerName string, username string, password string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.controllerCredentials == nil {
		s.controllerCredentials = map[string]controllerCredentials{
			controllerName: {
				username: username,
				password: password,
			},
		}
	} else {
		s.controllerCredentials[controllerName] = controllerCredentials{
			username: username,
			password: password,
		}
	}
	return nil
}
