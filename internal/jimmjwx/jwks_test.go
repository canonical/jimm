// Copyright 2024 Canonical.
package jimmjwx_test

import (
	"context"
	"encoding/json"
	"maps"
	"os"
	"testing"
	"time"

	qt "github.com/frankban/quicktest"
	"github.com/google/uuid"
	"github.com/lestrrat-go/jwx/v2/jwa"
	"github.com/lestrrat-go/jwx/v2/jwk"

	"github.com/canonical/jimm/v3/internal/jimmjwx"
)

func TestGenerateJWKS(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()

	jwks, privKeyPem := generateJWK(c)

	jwksIter := jwks.Keys(ctx)
	jwksIter.Next(ctx)
	key := jwksIter.Pair().Value.(jwk.Key)

	// kid
	_, err := uuid.Parse(key.KeyID())
	c.Assert(err, qt.IsNil)
	// use
	c.Assert(key.KeyUsage(), qt.Equals, "sig")
	// alg
	c.Assert(key.Algorithm(), qt.Equals, jwa.RS256)

	// It's fine for us to just test the key exists.
	c.Assert(string(privKeyPem), qt.Contains, "-----BEGIN RSA PRIVATE KEY-----")
}

func TestNewJWKSServiceParsesOperatorManagedConfig(t *testing.T) {
	c := qt.New(t)
	service, expectedSet := newJWKSService(c)
	set, err := service.Get(context.Background())
	c.Assert(err, qt.IsNil)
	c.Assert(set.Len(), qt.Equals, expectedSet.Len())
	c.Assert(service.CacheMaxAge(), qt.Equals, int64(600))
	signingKey, err := service.SigningKey(context.Background())
	c.Assert(err, qt.IsNil)
	c.Assert(signingKey, qt.IsNotNil)
}

func TestNewJWKSServiceRejectsUnmatchedPrivateKey(t *testing.T) {
	c := qt.New(t)
	params, _, _ := newJWKSServiceParams(c)
	_, wrongPrivateKey := generateJWK(c)
	err := os.WriteFile(params.PrivateKeyPath, wrongPrivateKey, 0o600)
	c.Assert(err, qt.IsNil)
	_, err = jimmjwx.NewJWKSService(params)
	c.Assert(err, qt.ErrorMatches, "jwks does not contain the public key for the provided private key")
}

func TestNewJWKSServiceServesMultipleKeys(t *testing.T) {
	c := qt.New(t)
	params, _, _ := newJWKSServiceParams(c)
	var document struct {
		Keys []map[string]any `json:"keys"`
	}
	rawJWKS, err := os.ReadFile(params.JWKSPath)
	c.Assert(err, qt.IsNil)
	err = json.Unmarshal(rawJWKS, &document)
	c.Assert(err, qt.IsNil)
	duplicateKey := make(map[string]any, len(document.Keys[0]))
	maps.Copy(duplicateKey, document.Keys[0])
	duplicateKey["kid"] = "previous-key"
	document.Keys = append(document.Keys, duplicateKey)
	rawJWKS, err = json.Marshal(document)
	c.Assert(err, qt.IsNil)
	err = os.WriteFile(params.JWKSPath, rawJWKS, 0o600)
	c.Assert(err, qt.IsNil)
	service, err := jimmjwx.NewJWKSService(params)
	c.Assert(err, qt.IsNil)
	set, err := service.Get(context.Background())
	c.Assert(err, qt.IsNil)
	c.Assert(set.Len(), qt.Equals, 2)
}

func TestNewJWKSServiceRejectsInvalidCacheMaxAge(t *testing.T) {
	c := qt.New(t)
	params, _, _ := newJWKSServiceParams(c)
	params.CacheMaxAge = "nope"
	_, err := jimmjwx.NewJWKSService(params)
	c.Assert(err, qt.ErrorMatches, "parse jwks cache max age: .*")
}

func TestJWKSServiceRefreshesFilesAfterCacheExpiry(t *testing.T) {
	c := qt.New(t)
	params, initialSet, _ := newJWKSServiceParams(c)
	params.CacheMaxAge = "1"
	service, err := jimmjwx.NewJWKSService(params)
	c.Assert(err, qt.IsNil)
	defer func() { c.Assert(service.Close(), qt.IsNil) }()

	refreshedSet, refreshedPrivateKey := generateJWK(c)
	rawJWKS, err := json.Marshal(refreshedSet)
	c.Assert(err, qt.IsNil)
	err = os.WriteFile(params.JWKSPath, rawJWKS, 0o600)
	c.Assert(err, qt.IsNil)
	err = os.WriteFile(params.PrivateKeyPath, refreshedPrivateKey, 0o600)
	c.Assert(err, qt.IsNil)

	time.Sleep(1100 * time.Millisecond)

	set, err := service.Get(context.Background())
	c.Assert(err, qt.IsNil)
	c.Assert(firstKeyID(c, set), qt.Equals, firstKeyID(c, refreshedSet))
	c.Assert(firstKeyID(c, set), qt.Not(qt.Equals), firstKeyID(c, initialSet))

	signingKey, err := service.SigningKey(context.Background())
	c.Assert(err, qt.IsNil)
	c.Assert(signingKey.KeyID(), qt.Equals, firstKeyID(c, refreshedSet))
}

func TestJWKSServiceFallsBackToCachedValueOnRefreshFailure(t *testing.T) {
	c := qt.New(t)
	params, initialSet, _ := newJWKSServiceParams(c)
	params.CacheMaxAge = "1"
	service, err := jimmjwx.NewJWKSService(params)
	c.Assert(err, qt.IsNil)
	defer func() { c.Assert(service.Close(), qt.IsNil) }()

	err = os.WriteFile(params.JWKSPath, []byte("not-json"), 0o600)
	c.Assert(err, qt.IsNil)

	time.Sleep(1100 * time.Millisecond)

	set, err := service.Get(context.Background())
	c.Assert(err, qt.IsNil)
	c.Assert(firstKeyID(c, set), qt.Equals, firstKeyID(c, initialSet))

	signingKey, err := service.SigningKey(context.Background())
	c.Assert(err, qt.IsNil)
	c.Assert(signingKey.KeyID(), qt.Equals, firstKeyID(c, initialSet))
}

func firstKeyID(c *qt.C, set jwk.Set) string {
	c.Helper()
	ctx := context.Background()
	iter := set.Keys(ctx)
	c.Assert(iter.Next(ctx), qt.IsTrue)
	return iter.Pair().Value.(jwk.Key).KeyID()
}
