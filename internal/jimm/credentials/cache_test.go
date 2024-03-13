// Copyright 2024 canonical.

package credentials

import (
	"context"
	"errors"
	"testing"

	qt "github.com/frankban/quicktest"
	"github.com/lestrrat-go/jwx/v2/jwk"
	"github.com/stretchr/testify/mock"
)

func TestCacheSequence_GetJWKSHitsCache(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()
	store := &MockCredentialStore{}

	store.
		On("PutJWKS", mock.Anything, mock.Anything).Return(nil).
		On("GetJWKS", mock.Anything).Return(jwk.NewSet(), nil).Times(1) // This bails on the second call.

	cache := NewCachedCredentialStore(store, CachedCredentialStoreParams{})

	err := cache.PutJWKS(ctx, nil) // The argument itself is not important, so nil is passed.
	c.Assert(err, qt.IsNil)

	// First GetJWKS should hit the wrapped credential store.
	retrieved1, err := cache.GetJWKS(ctx)
	c.Assert(err, qt.IsNil)
	c.Assert(retrieved1, qt.IsNotNil)

	// Second GetJWKS should be returned from the cached value.
	retrieved2, err := cache.GetJWKS(ctx)
	c.Assert(err, qt.IsNil)
	c.Assert(retrieved2, qt.Equals, retrieved1) // asserting same pointers.

	store.AssertExpectations(c)
}

func TestCacheSequence_CleanupJWKSPurgesCache(t *testing.T) {
	// This test is to ensure that cached values are purged when CleanupJWKS is
	// called. First, we do a put-get sequence to fetch the value into the cache
	// and then call the clean-up method. The next get operation should delegate
	// to the wrapped store and return the same error.
	c := qt.New(t)
	ctx := context.Background()
	store := &MockCredentialStore{}

	store.
		On("PutJWKS", mock.Anything, mock.Anything).Return(nil).
		On("GetJWKS", mock.Anything).Return(jwk.NewSet(), nil).Times(1).
		On("CleanupJWKS", mock.Anything).Return(nil).
		On("GetJWKS", mock.Anything).Return(nil, errors.New("not found"))

	cache := NewCachedCredentialStore(store, CachedCredentialStoreParams{})

	err := cache.PutJWKS(ctx, nil) // The argument itself is not important, so nil is passed.
	c.Assert(err, qt.IsNil)

	retrieved1, err := cache.GetJWKS(ctx)
	c.Assert(err, qt.IsNil)
	c.Assert(retrieved1, qt.IsNotNil)

	err = cache.CleanupJWKS(ctx)
	c.Assert(err, qt.IsNil)

	retrieved2, err := cache.GetJWKS(ctx)
	c.Assert(err, qt.ErrorMatches, "not found")
	c.Assert(retrieved2, qt.IsNil)

	store.AssertExpectations(c)
}

func TestCacheSequence_PutJWKSPurgesCache(t *testing.T) {
	// This test is to ensure that cached values are purged whenever a put
	// operation takes place. First, we do a put-get sequence to fetch the value
	// into the cache and then repeat the put-get sequence which should yield the
	// latest value.
	c := qt.New(t)
	ctx := context.Background()
	store := &MockCredentialStore{}

	latestJWKS := jwk.NewSet() // Keeping a reference to it for later use in assertion.
	store.
		On("PutJWKS", mock.Anything, mock.Anything).Return(nil).Times(1).
		On("GetJWKS", mock.Anything).Return(jwk.NewSet(), nil).Times(1).
		On("PutJWKS", mock.Anything, mock.Anything).Return(nil).Times(1).
		On("GetJWKS", mock.Anything).Return(latestJWKS, nil).Times(1)

	cache := NewCachedCredentialStore(store, CachedCredentialStoreParams{})

	err := cache.PutJWKS(ctx, nil) // The argument itself is not important, so nil is passed.
	c.Assert(err, qt.IsNil)

	retrieved1, err := cache.GetJWKS(ctx)
	c.Assert(err, qt.IsNil)
	c.Assert(retrieved1, qt.IsNotNil)

	err = cache.PutJWKS(ctx, nil) // The argument itself is not important, so nil is passed.
	c.Assert(err, qt.IsNil)

	retrieved2, err := cache.GetJWKS(ctx)
	c.Assert(err, qt.IsNil)
	c.Assert(retrieved2, qt.IsNotNil)
	c.Assert(retrieved2, qt.Equals, latestJWKS)

	store.AssertExpectations(c)
}

func TestCacheSequence_JWKSExpires(t *testing.T) {
	// We simulate the cache expiry by calling the internal cache object's purge method.
	c := qt.New(t)
	ctx := context.Background()
	store := &MockCredentialStore{}

	store.
		On("PutJWKS", mock.Anything, mock.Anything).Return(nil).
		On("GetJWKS", mock.Anything).Return(jwk.NewSet(), nil).Times(1).
		On("GetJWKS", mock.Anything).Return(jwk.NewSet(), nil).Times(1)

	cache := NewCachedCredentialStore(store, CachedCredentialStoreParams{})

	err := cache.PutJWKS(ctx, nil) // The argument itself is not important, so nil is passed.
	c.Assert(err, qt.IsNil)

	retrieved1, err := cache.GetJWKS(ctx)
	c.Assert(err, qt.IsNil)
	c.Assert(retrieved1, qt.IsNotNil)

	// This should be returned from the cached value.
	retrieved2, err := cache.GetJWKS(ctx)
	c.Assert(err, qt.IsNil)
	c.Assert(retrieved2, qt.Equals, retrieved1)

	cache.jwksCache.Purge()

	// Second retrieved value must be different due to expiry.
	retrieved3, err := cache.GetJWKS(ctx)
	c.Assert(err, qt.IsNil)
	c.Assert(retrieved3, qt.IsNotNil)
	c.Assert(retrieved3, qt.Not(qt.Equals), retrieved1)

	store.AssertExpectations(c)
}

func TestCacheSequence_GetOAuthKeyHitsCache(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()
	store := &MockCredentialStore{}

	store.
		On("PutOAuthKey", mock.Anything, mock.Anything).Return(nil).
		On("GetOAuthKey", mock.Anything).Return([]byte{0}, nil).Times(1) // This bails on the second call.

	cache := NewCachedCredentialStore(store, CachedCredentialStoreParams{})

	err := cache.PutOAuthKey(ctx, nil) // The argument itself is not important, so nil is passed.
	c.Assert(err, qt.IsNil)

	// First GetOAuthKey should hit the wrapped credential store.
	retrieved1, err := cache.GetOAuthKey(ctx)
	c.Assert(err, qt.IsNil)
	c.Assert(retrieved1, qt.DeepEquals, []byte{0})

	// Second GetOAuthKey should be returned from the cached value.
	retrieved2, err := cache.GetOAuthKey(ctx)
	c.Assert(err, qt.IsNil)
	c.Assert(retrieved2, qt.DeepEquals, retrieved1)

	store.AssertExpectations(c)
}

func TestCacheSequence_PutOAuthKeyPurgesCache(t *testing.T) {
	// This test is to ensure that cached values are purged whenever a put
	// operation takes place. First, we do a put-get sequence to fetch the value
	// into the cache and then repeat the put-get sequence which should yield the
	// latest value.
	c := qt.New(t)
	ctx := context.Background()
	store := &MockCredentialStore{}

	latestOAuthKey := []byte{255} // Keeping a reference to it for later use in assertion.
	store.
		On("PutOAuthKey", mock.Anything, mock.Anything).Return(nil).Times(1).
		On("GetOAuthKey", mock.Anything).Return([]byte{0}, nil).Times(1).
		On("PutOAuthKey", mock.Anything, mock.Anything).Return(nil).Times(1).
		On("GetOAuthKey", mock.Anything).Return(latestOAuthKey, nil).Times(1)

	cache := NewCachedCredentialStore(store, CachedCredentialStoreParams{})

	err := cache.PutOAuthKey(ctx, nil) // The argument itself is not important, so nil is passed.
	c.Assert(err, qt.IsNil)

	retrieved1, err := cache.GetOAuthKey(ctx)
	c.Assert(err, qt.IsNil)
	c.Assert(retrieved1, qt.DeepEquals, []byte{0})

	err = cache.PutOAuthKey(ctx, nil) // The argument itself is not important, so nil is passed.
	c.Assert(err, qt.IsNil)

	retrieved2, err := cache.GetOAuthKey(ctx)
	c.Assert(err, qt.IsNil)
	c.Assert(retrieved2, qt.IsNotNil)
	c.Assert(retrieved2, qt.DeepEquals, latestOAuthKey)

	store.AssertExpectations(c)
}

func TestCacheSequence_OAuthKeyExpires(t *testing.T) {
	// We simulate the cache expiry by calling the internal cache object's purge method.
	c := qt.New(t)
	ctx := context.Background()
	store := &MockCredentialStore{}

	store.
		On("PutOAuthKey", mock.Anything, mock.Anything).Return(nil).
		On("GetOAuthKey", mock.Anything).Return([]byte{0}, nil).Times(1).
		On("GetOAuthKey", mock.Anything).Return([]byte{255}, nil).Times(1)

	cache := NewCachedCredentialStore(store, CachedCredentialStoreParams{})

	err := cache.PutOAuthKey(ctx, nil) // The argument itself is not important, so nil is passed.
	c.Assert(err, qt.IsNil)

	retrieved1, err := cache.GetOAuthKey(ctx)
	c.Assert(err, qt.IsNil)
	c.Assert(retrieved1, qt.DeepEquals, []byte{0})

	// This should be returned from the cached value.
	retrieved2, err := cache.GetOAuthKey(ctx)
	c.Assert(err, qt.IsNil)
	c.Assert(retrieved2, qt.DeepEquals, retrieved1)

	cache.oauthKeyCache.Purge()

	// Second retrieved value must be different due to expiry.
	retrieved3, err := cache.GetOAuthKey(ctx)
	c.Assert(err, qt.IsNil)
	c.Assert(retrieved3, qt.DeepEquals, []byte{255})

	store.AssertExpectations(c)
}
