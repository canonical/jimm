// Copyright 2024 Canonical.
package jimmjwx_test

import (
	"context"
	"encoding/json"
	"maps"
	"testing"

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
	c.Assert(service.SigningKey(), qt.IsNotNil)
}

func TestNewJWKSServiceRejectsUnmatchedPrivateKey(t *testing.T) {
	c := qt.New(t)
	params, _, _ := newJWKSServiceParams(c)
	_, wrongPrivateKey := generateJWK(c)
	params.PrivateKeyPEM = string(wrongPrivateKey)
	_, err := jimmjwx.NewJWKSService(params)
	c.Assert(err, qt.ErrorMatches, "jwks does not contain the public key for the provided private key")
}

func TestNewJWKSServiceServesMultipleKeys(t *testing.T) {
	c := qt.New(t)
	params, _, _ := newJWKSServiceParams(c)
	var document struct {
		Keys []map[string]any `json:"keys"`
	}
	err := json.Unmarshal([]byte(params.JWKS), &document)
	c.Assert(err, qt.IsNil)
	duplicateKey := make(map[string]any, len(document.Keys[0]))
	maps.Copy(duplicateKey, document.Keys[0])
	duplicateKey["kid"] = "previous-key"
	document.Keys = append(document.Keys, duplicateKey)
	rawJWKS, err := json.Marshal(document)
	c.Assert(err, qt.IsNil)
	params.JWKS = string(rawJWKS)
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
