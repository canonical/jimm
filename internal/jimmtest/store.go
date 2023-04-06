package jimmtest

import (
	"context"
	"sync"
	"time"

	"github.com/CanonicalLtd/jimm/internal/errors"
	"github.com/CanonicalLtd/jimm/internal/jimm/credentials"
	"github.com/lestrrat-go/jwx/v2/jwk"
)

// InMemoryCredentialStore implements CredentialStore but only implements
// JWKS methods in order to use it as an in memory credential store replacing
// vault for tests.
type InMemoryCredentialStore struct {
	credentials.CredentialStore

	mu         sync.RWMutex
	jwks       jwk.Set
	privateKey []byte
	expiry     time.Time
}

// CleanupJWKS removes all secrets associated with the JWKS process.
func (s *InMemoryCredentialStore) CleanupJWKS(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.jwks = nil
	s.privateKey = nil
	s.expiry = time.Time{}

	return nil
}

// GetJWKS returns the current key set stored within the credential store.
func (s *InMemoryCredentialStore) GetJWKS(ctx context.Context) (jwk.Set, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.jwks == nil {
		return nil, errors.E(errors.CodeNotFound)
	}
	jwks := s.jwks
	return jwks, nil
}

// GetJWKSPrivateKey returns the current private key for the active JWKS.
func (s *InMemoryCredentialStore) GetJWKSPrivateKey(ctx context.Context) ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.privateKey == nil || len(s.privateKey) == 0 {
		return nil, errors.E(errors.CodeNotFound)
	}

	pk := make([]byte, len(s.privateKey))
	copy(pk, s.privateKey)

	return pk, nil
}

// GetJWKSExpiry returns the expiry of the active JWKS.
func (s *InMemoryCredentialStore) GetJWKSExpiry(ctx context.Context) (time.Time, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.expiry.IsZero() {
		return time.Time{}, errors.E(errors.CodeNotFound)
	}

	return s.expiry, nil
}

// PutJWKS puts a generated RS256[4096 bit] JWKS without x5c or x5t into the credential store.
func (s *InMemoryCredentialStore) PutJWKS(ctx context.Context, jwks jwk.Set) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.jwks = jwks

	return nil
}

// PutJWKSPrivateKey persists the private key associated with the current JWKS within the store.
func (s *InMemoryCredentialStore) PutJWKSPrivateKey(ctx context.Context, pem []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.privateKey = make([]byte, len(pem))
	copy(s.privateKey, pem)

	return nil
}

// PutJWKSExpiry sets the expiry time for the current JWKS within the store.
func (s *InMemoryCredentialStore) PutJWKSExpiry(ctx context.Context, expiry time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.expiry = expiry

	return nil
}
