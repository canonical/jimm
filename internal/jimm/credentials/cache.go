// Copyright 2024 canonical.

package credentials

import (
	"context"
	"sync"
	"time"

	"github.com/hashicorp/golang-lru/v2/expirable"
	"github.com/lestrrat-go/jwx/v2/jwk"
)

// CachedCredentialStore is a transparent wrapper around a credential store
// instance to enable caching of secrets/credentials.
//
// Note that not all kinds of secrets/credentials are cached, which is why not
// all `CredentialStore` methods are wrapped.
type CachedCredentialStore struct {
	CredentialStore

	jwksCache     *expirable.LRU[string, jwk.Set]
	jwksMutex     sync.Mutex
	oauthKeyCache *expirable.LRU[string, []byte]
	oauthKeyMutex sync.Mutex
}

// defaultJWKSExpiry default for CachedCredentialStoreParams.JWKExpiry field value.
const defaultJWKSExpiry time.Duration = time.Duration(1 * time.Hour)

// defaultOAuthKeyExpiry default for CachedCredentialStoreParams.OAuthKeyExpiry field value.
const defaultOAuthKeyExpiry time.Duration = time.Duration(1 * time.Hour)

// CachedCredentialStoreParams contains configuration parameters to tune new
// CachedCredentialStore instances.
type CachedCredentialStoreParams struct {
	// JWKSExpiry retention period for JWKS data. If this is set to zero, the
	// value will be taken from `defaultJWKSExpiry`.
	//
	// Note that the default  duration is configured at 1h, which should be much
	// lower than the rotation period of the JWK set.
	JWKSExpiry time.Duration

	// OAuthKeyExpiry retention period for OAuth session signing key. If this is
	// set to zero, the value will be taken from the `defaultOAuthKeyExpiry`.
	OAuthKeyExpiry time.Duration
}

// NewCachedCredentialStore creates a new CachedCredentialStore used for storing
// and periodically fetching credentials/secrets from a given credential store.
// Note that
func NewCachedCredentialStore(store CredentialStore, params CachedCredentialStoreParams) CachedCredentialStore {
	jwksExpiry := params.JWKSExpiry
	if jwksExpiry == 0 {
		jwksExpiry = defaultJWKSExpiry
	}

	oauthKeyExpiry := params.OAuthKeyExpiry
	if oauthKeyExpiry == 0 {
		oauthKeyExpiry = defaultOAuthKeyExpiry
	}

	return CachedCredentialStore{
		CredentialStore: store,
		jwksCache:       expirable.NewLRU[string, jwk.Set](1, nil, jwksExpiry),
		oauthKeyCache:   expirable.NewLRU[string, []byte](1, nil, oauthKeyExpiry),
	}
}

const jwksCacheKey = "jwks"
const oauthKeyCacheKey = "oauthKey"

// CleanupJWKS cleans up the JWKS cache and delegates to the wrapped store.
func (c *CachedCredentialStore) CleanupJWKS(ctx context.Context) error {
	c.jwksMutex.Lock()
	defer c.jwksMutex.Unlock()

	c.jwksCache.Purge()
	return c.CredentialStore.CleanupJWKS(ctx)
}

// GetJWKS returns the cached return value of the last call to the wrapped store's
// corresponding method. If there is no cached value it delegates the call to the
// wrapped store and then caches the returned value.
func (c *CachedCredentialStore) GetJWKS(ctx context.Context) (jwk.Set, error) {
	c.jwksMutex.Lock()
	defer c.jwksMutex.Unlock()

	if val, ok := c.jwksCache.Get(jwksCacheKey); ok {
		return val, nil
	}
	ks, err := c.CredentialStore.GetJWKS(ctx)
	if err != nil {
		return nil, err
	}
	c.jwksCache.Add(jwksCacheKey, ks)
	return ks, nil
}

// PutJWKS cleans up the JWKS cache and delegates to the wrapped store.
func (c *CachedCredentialStore) PutJWKS(ctx context.Context, jwks jwk.Set) error {
	c.jwksMutex.Lock()
	defer c.jwksMutex.Unlock()

	// TODO(babakks): Note that we're cleaning up the entire cache which is fine
	// for now, because at the moment there's only one entry (i.e., a JWK set) in
	// the cache. In future, if we had more JWKSs in the cache, we need to just
	// remove the one that is being modified.
	c.jwksCache.Purge()

	return c.CredentialStore.PutJWKS(ctx, jwks)
}

// GetOAuthKey returns the cached return value of the last call to the wrapped store's
// corresponding method. If there is no cached value it delegates the call to the
// wrapped store and then caches the returned value.
func (c *CachedCredentialStore) GetOAuthKey(ctx context.Context) ([]byte, error) {
	c.oauthKeyMutex.Lock()
	defer c.oauthKeyMutex.Unlock()

	if val, ok := c.oauthKeyCache.Get(oauthKeyCacheKey); ok {
		return val, nil
	}
	key, err := c.CredentialStore.GetOAuthKey(ctx)
	if err != nil {
		return nil, err
	}
	c.oauthKeyCache.Add(oauthKeyCacheKey, key)
	return key, nil
}

// PutOAuthKey cleans up the OAuth key cache and delegates to the wrapped store.
func (c *CachedCredentialStore) PutOAuthKey(ctx context.Context, raw []byte) error {
	c.oauthKeyMutex.Lock()
	defer c.oauthKeyMutex.Unlock()

	c.oauthKeyCache.Purge()
	return c.CredentialStore.PutOAuthKey(ctx, raw)
}
