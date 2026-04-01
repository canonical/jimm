// Copyright 2024 Canonical.
package jimmjwx

import (
	"context"
	"encoding/json"
	"maps"
	"os"
	"testing"
	"testing/synctest"
	"time"

	qt "github.com/frankban/quicktest"
	"github.com/juju/zaputil/zapctx"
	"github.com/lestrrat-go/jwx/v2/jwk"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestNewJWKSServiceParsesOperatorManagedConfig(t *testing.T) {
	c := qt.New(t)
	service, expectedSet := newJWKSService(c)
	set, err := service.Get(context.Background())
	c.Assert(err, qt.IsNil)
	c.Assert(set.Len(), qt.Equals, expectedSet.Len())
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
	_, err = NewJWKSService(c.Context(), params)
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
	service, err := NewJWKSService(c.Context(), params)
	c.Assert(err, qt.IsNil)
	set, err := service.Get(context.Background())
	c.Assert(err, qt.IsNil)
	c.Assert(set.Len(), qt.Equals, 2)
}

func TestJWKSServiceRefreshesFilesAfterCacheExpiry(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c := qt.New(t)
		params, initialSet, _ := newJWKSServiceParams(c)
		service, err := NewJWKSService(c.Context(), params)
		c.Assert(err, qt.IsNil)

		refreshedSet, refreshedPrivateKey := generateJWK(c)
		rawJWKS, err := json.Marshal(refreshedSet)
		c.Assert(err, qt.IsNil)
		err = os.WriteFile(params.JWKSPath, rawJWKS, 0o600)
		c.Assert(err, qt.IsNil)
		err = os.WriteFile(params.PrivateKeyPath, refreshedPrivateKey, 0o600)
		c.Assert(err, qt.IsNil)

		time.Sleep(jwksRefreshInterval + time.Minute)

		set, err := service.Get(context.Background())
		c.Assert(err, qt.IsNil)
		c.Assert(firstKeyID(c, set), qt.Equals, firstKeyID(c, refreshedSet))
		c.Assert(firstKeyID(c, set), qt.Not(qt.Equals), firstKeyID(c, initialSet))

		signingKey, err := service.SigningKey(context.Background())
		c.Assert(err, qt.IsNil)
		c.Assert(signingKey.KeyID(), qt.Equals, firstKeyID(c, refreshedSet))
	})
}

func TestJWKSServiceFallsBackToCachedValueOnRefreshFailure(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c := qt.New(t)
		params, initialSet, _ := newJWKSServiceParams(c)
		service, err := NewJWKSService(c.Context(), params)
		c.Assert(err, qt.IsNil)

		err = os.WriteFile(params.JWKSPath, []byte("not-json"), 0o600)
		c.Assert(err, qt.IsNil)

		time.Sleep(jwksRefreshInterval + time.Minute)

		set, err := service.Get(context.Background())
		c.Assert(err, qt.IsNil)
		c.Assert(firstKeyID(c, set), qt.Equals, firstKeyID(c, initialSet))

		signingKey, err := service.SigningKey(context.Background())
		c.Assert(err, qt.IsNil)
		c.Assert(signingKey.KeyID(), qt.Equals, firstKeyID(c, initialSet))
	})
}

func TestJWKSServiceLogsWhenRefreshLoopContextIsCancelled(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c := qt.New(t)
		params, _, _ := newJWKSServiceParams(c)

		core, logs := observer.New(zap.InfoLevel)
		ctx := zapctx.WithLogger(context.Background(), zap.New(core))
		ctx, cancel := context.WithCancel(ctx)

		_, err := NewJWKSService(ctx, params)
		c.Assert(err, qt.IsNil)

		cancel()
		synctest.Wait()

		c.Assert(logs.Len(), qt.Equals, 1)
		c.Assert(logs.All()[0].Message, qt.Equals, "exiting jwks refresh polling")
	})
}

func firstKeyID(c *qt.C, set jwk.Set) string {
	c.Helper()
	ctx := context.Background()
	iter := set.Keys(ctx)
	c.Assert(iter.Next(ctx), qt.IsTrue)
	return iter.Pair().Value.(jwk.Key).KeyID()
}
