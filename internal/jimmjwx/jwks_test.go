// Copyright 2024 Canonical.
package jimmjwx_test

import (
	"context"
	"os"
	"testing"
	"testing/synctest"
	"time"

	qt "github.com/frankban/quicktest"
	"github.com/google/uuid"
	"github.com/lestrrat-go/jwx/v2/jwa"
	"github.com/lestrrat-go/jwx/v2/jwk"

	"github.com/canonical/jimm/v3/internal/jimmjwx"
)

func TestMain(m *testing.M) {
	code := m.Run()
	os.Exit(code)
}

func TestGenerateJWKS(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()

	jwks, privKeyPem, err := jimmjwx.GenerateJWK(ctx)
	c.Assert(err, qt.IsNil)

	jwksIter := jwks.Keys(ctx)
	jwksIter.Next(ctx)
	key := jwksIter.Pair().Value.(jwk.Key)

	// kid
	_, err = uuid.Parse(key.KeyID())
	c.Assert(err, qt.IsNil)
	// use
	c.Assert(key.KeyUsage(), qt.Equals, "sig")
	// alg
	c.Assert(key.Algorithm(), qt.Equals, jwa.RS256)

	// It's fine for us to just test the key exists.
	c.Assert(string(privKeyPem), qt.Contains, "-----BEGIN RSA PRIVATE KEY-----")
}

// This test is difficult to gauge, as it is truly only time based.
// As such, it will retry 60 times on a 500ms basis.
func TestStartJWKSRotatorWithNoJWKSInTheStore(t *testing.T) {
	c := qt.New(t)
	ctx, cancelCtx := context.WithCancel(context.Background())
	defer cancelCtx()

	store := newStore(c)
	err := store.CleanupJWKS(ctx)
	c.Assert(err, qt.IsNil)
	svc := jimmjwx.NewJWKSService(store)
	startAndTestRotator(c, ctx, store, svc)
}

// Due to the nature of this test, we do not test exact times (as it will vary drastically machine to machine)
// But rather just ensure the JWKS has infact updated.
//
// So I suppose this test is "best effort", but will only ever pass if the code is truly OK.
func TestStartJWKSRotatorRotatesAJWKS(t *testing.T) {
	c := qt.New(t)
	ctx, cancelCtx := context.WithCancel(context.Background())
	defer cancelCtx()
	store := newStore(c)
	err := store.CleanupJWKS(ctx)
	c.Assert(err, qt.IsNil)

	svc := jimmjwx.NewJWKSService(store)

	// So, we first put a fresh JWKS in the store
	err = store.PutJWKS(ctx, getJWKS(c))
	c.Check(err, qt.IsNil)

	// Get the key we're aware of right now
	ks, err := store.GetJWKS(ctx)
	c.Assert(err, qt.IsNil)
	initialKey, ok := ks.Key(0)
	c.Assert(ok, qt.IsTrue)

	// Start up the rotator
	err = svc.StartJWKSRotator(ctx, time.NewTicker(time.Second).C, time.Now())
	c.Assert(err, qt.IsNil)
	// We retry 500ms * 60 (30s) to test the diff
	for i := 0; i < 60; i++ {
		time.Sleep(500 * time.Millisecond)
		ks2, err := store.GetJWKS(ctx)
		c.Assert(err, qt.IsNil)
		newKey, ok := ks2.Key(0)
		c.Assert(ok, qt.IsTrue)
		if initialKey.KeyID() != newKey.KeyID() {
			break
		}
	}
}

func TestStartJWKSRotatorV2_InitialisesFromLegacy(t *testing.T) {
	c := qt.New(t)
	ctx, cancelCtx := context.WithCancel(context.Background())
	defer cancelCtx()

	store := setupCredentialStore(ctx, c)
	err := store.CleanupJWKS(ctx)
	c.Assert(err, qt.IsNil)

	legacyJWKS, legacyPrivateKey, err := jimmjwx.GenerateJWK(ctx)
	c.Assert(err, qt.IsNil)

	err = store.PutJWKS(ctx, legacyJWKS)
	c.Assert(err, qt.IsNil)
	err = store.PutJWKSPrivateKey(ctx, legacyPrivateKey)
	c.Assert(err, qt.IsNil)

	legacyKey, ok := legacyJWKS.Key(0)
	c.Assert(ok, qt.IsTrue)

	svc := jimmjwx.NewJWKSService(store)
	err = svc.StartJWKSRotatorV2(ctx, jimmjwx.StartJWKSRotatorParams{
		CheckRotateRequired: make(chan time.Time),
		RotationInterval:    24 * time.Hour,
		GracePeriod:         15 * time.Minute,
		MaxTokenLifetime:    time.Hour,
	})
	c.Assert(err, qt.IsNil)

	meta, err := store.GetKeyMetadata(ctx)
	c.Assert(err, qt.IsNil)
	c.Assert(meta.CurrentActiveKID, qt.Equals, legacyKey.KeyID())
	c.Assert(meta.Keys, qt.HasLen, 1)
	c.Assert(meta.Keys[0].KID, qt.Equals, legacyKey.KeyID())
	c.Assert(meta.Keys[0].PrivateKey, qt.Equals, string(legacyPrivateKey))
	c.Assert(meta.Keys[0].ActivatedAt, qt.IsNotNil)
}

// This test tests the rotator from an empty state, into a full rotation verifying that the grace
// period is honoured.
func TestStartJWKSRotatorV2_FullRunFromUninitialised(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c := qt.New(t)
		ctx, cancelCtx := context.WithCancel(context.Background())
		defer cancelCtx()

		store := setupCredentialStore(ctx, c)

		svc := jimmjwx.NewJWKSService(store)
		checkRotateRequired := make(chan time.Time, 1)
		p := jimmjwx.StartJWKSRotatorParams{
			CheckRotateRequired: checkRotateRequired,
			RotationInterval:    24 * time.Hour,
			GracePeriod:         15 * time.Minute,
			// Keep old key publishable beyond rotation interval so we can observe
			// old(active)+new(pre-active) overlap.
			MaxTokenLifetime: 48 * time.Hour,
		}
		err := svc.StartJWKSRotatorV2(ctx, p)
		c.Assert(err, qt.IsNil)

		// Fresh init should create one key and make it active immediately.
		meta, err := store.GetKeyMetadata(ctx)
		c.Assert(err, qt.IsNil)
		// Expect just the one.
		c.Assert(meta.Keys, qt.HasLen, 1)
		// And it is active and good to go.
		c.Assert(meta.CurrentActiveKID, qt.Equals, meta.Keys[0].KID)
		c.Assert(meta.Keys[0].KID, qt.Not(qt.Equals), "")
		c.Assert(meta.Keys[0].PrivateKey, qt.Contains, "BEGIN RSA PRIVATE KEY")
		c.Assert(meta.Keys[0].PublicJWK, qt.Not(qt.Equals), "")
		c.Assert(meta.Keys[0].ActivatedAt, qt.IsNotNil)
		c.Assert(meta.RotationInterval, qt.Equals, 24*time.Hour)
		c.Assert(meta.GracePeriod, qt.Equals, 15*time.Minute)
		c.Assert(meta.MaxTokenLifetime, qt.Equals, 48*time.Hour)

		ogKID := meta.CurrentActiveKID

		// Now we fast forward over the rotation period, but before the next key is set active. (grace peroiod)
		time.Sleep(p.RotationInterval + time.Second)
		checkRotateRequired <- time.Now()
		synctest.Wait()

		meta, err = store.GetKeyMetadata(ctx)
		c.Assert(err, qt.IsNil)
		// We should have two keys now, one active and one preactive and we expect
		// the OG key to be the active one still, as we are in the grace period.
		c.Assert(meta.Keys, qt.HasLen, 2)
		c.Assert(meta.CurrentActiveKID, qt.Equals, ogKID)

		expectedNewActiveKID := ""
		// Retrieve the preactive key to check it is defo the active key after retirement.
		for _, key := range meta.Keys {
			if key.KID != ogKID {
				expectedNewActiveKID = key.KID
				break
			}
		}

		// Now wefast forward to just after the original key expiry (ActivatedAt + max token lifetime + grace).
		// We subtract the first jump we already did, and add a small epsilon so retirement condition is strictly true.
		// This avoids usovershooting into another rotation window.
		// For reference: (24hr + 15m) - (24hr + 1s) + 500ms = 14m59s500ms, and we jumped rotate +1second.
		// As such we're 500ms after the retirement condition is satisfied, but not so far as to have triggered another rotation window.
		secondJump := (p.MaxTokenLifetime + p.GracePeriod) - (p.RotationInterval + time.Second) + 500*time.Millisecond
		time.Sleep(secondJump)
		checkRotateRequired <- time.Now()
		synctest.Wait()

		meta, err = store.GetKeyMetadata(ctx)
		c.Assert(err, qt.IsNil)
		// We should have just the one key now, and it should be the new one.
		c.Assert(meta.Keys, qt.HasLen, 1)
		c.Assert(meta.CurrentActiveKID, qt.Equals, expectedNewActiveKID)
	})
}
