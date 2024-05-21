package jimmtest

import (
	"context"
	"sync"
	"time"

	"github.com/juju/names/v5"
	"github.com/lestrrat-go/jwx/v2/jwk"

	"github.com/canonical/jimm/internal/errors"
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
	jwks                      jwk.Set
	privateKey                []byte
	expiry                    time.Time
	oauthKey                  []byte
	controllerCredentials     map[string]controllerCredentials
	cloudCredentialAttributes map[string]map[string]string
}

// NewInMemoryCredentialStore returns a new instance of `InMemoryCredentialStore`
// with some secrets/keys being populated.
func NewInMemoryCredentialStore() *InMemoryCredentialStore {
	return &InMemoryCredentialStore{
		oauthKey: []byte(JWTTestSecret),
	}
}

// Get retrieves the stored attributes of a cloud credential.
func (s *InMemoryCredentialStore) Get(ctx context.Context, credTag names.CloudCredentialTag) (map[string]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	attrs, ok := s.cloudCredentialAttributes[credTag.String()]
	if !ok {
		return nil, errors.E(errors.CodeNotFound)
	}
	attrsCopy := make(map[string]string, len(attrs))
	for k, v := range attrs {
		attrsCopy[k] = v
	}
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
	for k, v := range attrs {
		attrsCopy[k] = v
	}
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
		return "", "", errors.E(errors.CodeNotFound)
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
			controllerName: controllerCredentials{
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

// CleanupOAuthSecrets removes all secrets associated with OAuth.
func (s *InMemoryCredentialStore) CleanupOAuthSecrets(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.oauthKey = nil
	return nil
}

// GetOAuthSecret returns the current HS256 (symmetric encryption) secret used to sign OAuth session tokens.
func (s *InMemoryCredentialStore) GetOAuthSecret(ctx context.Context) ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.oauthKey == nil || len(s.oauthKey) == 0 {
		return nil, errors.E(errors.CodeNotFound)
	}

	key := make([]byte, len(s.oauthKey))
	copy(key, s.oauthKey)

	return key, nil
}

// PutOAuthSecret puts a HS256 (symmetric encryption) secret into the credentials store for signing OAuth session tokens.
func (s *InMemoryCredentialStore) PutOAuthSecret(ctx context.Context, raw []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.oauthKey = make([]byte, len(raw))
	copy(s.oauthKey, raw)

	return nil
}
