// Copyright 2025 Canonical.

package jimmjwx

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/juju/zaputil/zapctx"
	"github.com/lestrrat-go/jwx/v2/jwa"
	"github.com/lestrrat-go/jwx/v2/jwk"
	"go.uber.org/zap"

	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/vault"
)

// CredentialStore defines the interface for a store that can manage
// JSON Web Key Sets (JWKS), their associated private keys, expiry times
// and cleanup operations. This is used by the JWKSService to manage
// the JWKS lifecycle for JIMM.
type CredentialStore interface {
	// CleanupJWKS removes all secrets associated with the JWKS process.
	CleanupJWKS(ctx context.Context) error

	// GetJWKS returns the current key set stored within the credential store.
	GetJWKS(ctx context.Context) (jwk.Set, error)

	// GetJWKSPrivateKey returns the current private key for the active JWKS
	GetJWKSPrivateKey(ctx context.Context) ([]byte, error)

	// GetJWKSExpiry returns the expiry of the active JWKS.
	GetJWKSExpiry(ctx context.Context) (time.Time, error)

	// PutJWKS puts a generated RS256[4096 bit] JWKS without x5c or x5t into the credential store.
	PutJWKS(ctx context.Context, jwks jwk.Set) error

	// PutJWKSPrivateKey persists the private key associated with the current JWKS within the store.
	PutJWKSPrivateKey(ctx context.Context, pem []byte) error

	// PutJWKSExpiry sets the expiry time for the current JWKS within the store.
	PutJWKSExpiry(ctx context.Context, expiry time.Time) error

	// GetKeyMetadata retrieves the key metadata for rotation tracking.
	GetKeyMetadata(ctx context.Context) (vault.KeyMetadata, error)

	// PutKeyMetadata stores the key metadata for rotation tracking.
	PutKeyMetadata(ctx context.Context, metadata vault.KeyMetadata) error
}

// JWKSService handles the creation, rotation and retrieval of JWKS for JIMM.
// It utilises the underlying credential store currently in effect.
type JWKSService struct {
	credentialStore CredentialStore
}

// NewJWKSService returns a new JWKS service for handling JIMMs JWKS.
func NewJWKSService(credStore CredentialStore) *JWKSService {
	return &JWKSService{credentialStore: credStore}
}

func (jwks *JWKSService) StartJWKSRotator(ctx context.Context, checkRotateRequired <-chan time.Time, initialRotateRequiredTime time.Time) error {

	credStore := jwks.credentialStore

	if err := rotateJWKS(ctx, credStore, initialRotateRequiredTime); err != nil {
		return errors.E(fmt.Errorf("rotate jwks: %w", err))
	}

	// The rotation method is as follows, if an expiry is not present, we know
	// this is the first attempt to set the initial JWKS (or it may be subsequent from erroneous attempts).
	// As the next attempt comes around, it is a simple check if the times is after the current.
	//
	// In this case we generate a new set, which should expire in 3 months.
	go func() {
		for {
			select {
			case <-checkRotateRequired:
				if err := rotateJWKS(ctx, credStore, initialRotateRequiredTime); err != nil {
					zapctx.Error(ctx, "security failure", zap.NamedError("jwks-error", err))
				}
			case <-ctx.Done():
				zapctx.Debug(ctx, "shutdown for JWKS rotator complete.")
				return
			}
		}
	}()

	return nil
}

// generateJWKS generates a new set of JWK using RSA256[4096]
//
// It will return a jwk.Set containing the public key
// and a PEM encoded private key for JWT signing.
func generateJWK(ctx context.Context) (jwk.Set, []byte, error) {

	// Due to the sensitivity of controllers, it is best we allow a larger encryption bit size
	// and accept any negligible wire cost.
	keySet, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		return nil, nil, errors.E(err)
	}

	privateKeyPEM := pem.EncodeToMemory(
		&pem.Block{
			Type:  "RSA PRIVATE KEY",
			Bytes: x509.MarshalPKCS1PrivateKey(keySet),
		},
	)

	// We also use the same methodology of generating UUIDs for our KID
	kid, err := uuid.NewRandom()
	if err != nil {
		return nil, nil, errors.E(err)
	}

	jwks, err := jwk.FromRaw(keySet.PublicKey)
	if err != nil {
		return nil, nil, errors.E(err)
	}
	err = jwks.Set(jwk.KeyIDKey, kid.String())
	if err != nil {
		return nil, nil, errors.E(err)
	}

	err = jwks.Set(jwk.KeyUsageKey, "sig") // Couldn't find const for this...
	if err != nil {
		return nil, nil, errors.E(err)
	}

	err = jwks.Set(jwk.AlgorithmKey, jwa.RS256)
	if err != nil {
		return nil, nil, errors.E(err)
	}

	ks := jwk.NewSet()
	err = ks.AddKey(jwks)
	if err != nil {
		return nil, nil, errors.E(err)
	}

	return ks, privateKeyPEM, nil
}

// rotateJWKS is the legacy rotation function. Use rotateKeysV2 instead.
func rotateJWKS(ctx context.Context, credStore CredentialStore, initialExpiryTime time.Time) error {
	// putJwks simply attempts the process of setting up the JWKS suite
	// and all secrets required for JIMM to sign JWTs and clients to verify
	// JWTs from JIMM.
	putJwks := func(expires time.Time) error {
		set, key, err := generateJWK(ctx)
		if err != nil {
			return err
		}

		err = credStore.PutJWKS(ctx, set)
		if err != nil {
			return err
		}

		err = credStore.PutJWKSPrivateKey(ctx, key)
		if err != nil {
			return err
		}

		err = credStore.PutJWKSExpiry(ctx, expires)
		if err != nil {
			return err
		}

		zapctx.Debug(ctx, "set a new JWKS", zap.String("expiry", expires.String()))
		return nil
	}

	expires, err := credStore.GetJWKSExpiry(ctx)
	if err != nil {
		zapctx.Debug(ctx, "failed to get expiry", zap.Error(err))
		zapctx.Debug(ctx, "setting initial expiry", zap.Time("time", initialExpiryTime))
		err = putJwks(initialExpiryTime)
		if err != nil {
			if jwksErr := credStore.CleanupJWKS(ctx); jwksErr != nil {
				zapctx.Error(ctx, "failed to cleanup jwks", zap.Error(jwksErr))
			}
			return errors.E(fmt.Errorf("failed to put JWKS: %w", err))
		}
	} else {
		// Check it has expired.
		now := time.Now().UTC()
		if now.After(expires) {
			// In theory, an error should not happen anymore as the necessary
			// components exist from the previous failed expiry attempt.
			err = putJwks(time.Now().UTC().AddDate(0, 3, 0))
			if err != nil {
				if jwksErr := credStore.CleanupJWKS(ctx); jwksErr != nil {
					zapctx.Error(ctx, "failed to cleanup jwks", zap.Error(jwksErr))
				}
				return errors.E(fmt.Errorf("failed to put JWKS: %w", err))
			}
			zapctx.Debug(ctx, "set a new JWKS", zap.String("expiry", expires.String()))
		}
	}
	return nil
}

// StartJWKSRotatorParams holds the params to start the JWKS rotator.
type StartJWKSRotatorParams struct {
	CheckRotateRequired <-chan time.Time
	RotationInterval    time.Duration
	GracePeriod         time.Duration
	MaxTokenLifetime    time.Duration
}

// StartJWKSRotatorV2 starts the new key rotation routine using the Zalando pattern.
// It periodically checks if rotation is due and performs the full rotation cycle.
//
// We also currently don't use x5c and x5t for validation and expect users
// to use e and n for validation.
// https://stackoverflow.com/questions/61395261/how-to-validate-signature-of-jwt-from-jwks-without-x5c
func (jwks *JWKSService) StartJWKSRotatorV2(
	ctx context.Context,
	p StartJWKSRotatorParams,
) error {
	credStore := jwks.credentialStore

	// Perform initial rotation/migration
	if err := rotateKeysV2(ctx, credStore, p); err != nil {
		zapctx.Error(ctx, "initial rotation failed", zap.Error(err))
		return errors.E(fmt.Errorf("initial key rotation: %w", err))
	}

	// Start background rotation goroutine
	go func() {
		for {
			select {
			case <-p.CheckRotateRequired:
				if err := rotateKeysV2(ctx, credStore, p); err != nil {
					zapctx.Error(ctx, "key rotation failed", zap.Error(err))
				}
			case <-ctx.Done():
				zapctx.Debug(ctx, "JWKS rotator shutdown complete")
				return
			}
		}
	}()

	return nil
}

// rotateKeysV2 runs one rotation tick using a small state machine over
// KeyMetadata. The state is derived from timestamps and CurrentActiveKID:
//
// 1) Initialize/Migrate
//   - If metadata is empty, migrate legacy JWKS if present.
//   - If truly fresh, create an active key immediately (no initial grace wait).
//
// 2) Retire expired keys
//   - Remove keys with ActivatedAt + MaxTokenLifetime + GracePeriod < now.
//
// 3) Promote pre-active key
//   - If any key has ActivatedAt <= now and is not active, set it as active.
//
// 4) Generate pre-active key when rotation is due
//   - When the active key's ActivatedAt + RotationInterval < now, create a new
//     key with ActivatedAt = now + GracePeriod (overlap window).
//
// 5) Persist metadata
//   - Updated metadata is stored; JWKS serving and signing read from it.
func rotateKeysV2(ctx context.Context, credStore CredentialStore, p StartJWKSRotatorParams) error {
	now := time.Now()

	// Load existing metadata.
	meta, err := credStore.GetKeyMetadata(ctx)
	if err != nil {
		zapctx.Error(ctx, "failed to get key metadata", zap.Error(err))
		return errors.E(fmt.Errorf("get key metadata: %w", err))
	}

	// Check if metadata has been initialised, if it hasn't, we know
	// this is an older JIMM deployment without the newer graceful rotation implemented.
	// And we'll pull the old key information into the new format, and set it as active,
	// with the same ActivatedAt time as the old key.
	//
	// Once we know all older deployments should have rotated at least once,
	// we can remove the legacy migrate and just start with an empty metadata
	// and set the defaults gor grace, rotate, and token TTL.
	if !metadataInitialised(&meta) {
		if err := migrateFromLegacyKeys(ctx, credStore, &meta, p.RotationInterval, p.GracePeriod, p.MaxTokenLifetime); err != nil {
			zapctx.Error(ctx, "failed to migrate legacy keys", zap.Error(err))
			return errors.E(fmt.Errorf("migrate legacy keys: %w", err))
		}
	}

	// Retire keys that have expired.
	retireExpiredKeys(&meta, now)

	// Promote any keys that were previously set to preactive.
	promotePreActiveKey(&meta, now)

	// Generate a preactive key if rotation has passed.
	if needsRotation(&meta, now) {
		if err := generatePreActiveKey(&meta, now); err != nil {
			return errors.E(fmt.Errorf("generate pre-active key: %w", err))
		}
	}

	// Update metadata.
	if err := credStore.PutKeyMetadata(ctx, meta); err != nil {
		return errors.E(fmt.Errorf("put key metadata: %w", err))
	}

	zapctx.Debug(ctx,
		"key rotation cycle complete",
		zap.String("active_kid", meta.CurrentActiveKID),
		zap.Int("total_keys", len(meta.Keys)),
	)

	return nil
}

// migrateFromLegacyKeys reads the existing JWKS and private key from legacy storage
// and converts them to the new KeyMetadata format.
func migrateFromLegacyKeys(
	ctx context.Context,
	credStore CredentialStore,
	meta *vault.KeyMetadata,
	defaultRotationInterval,
	defaultGracePeriod,
	defaultMaxTokenLifetime time.Duration,
) error {
	// Get old JWKS
	legacyJWKS, err := credStore.GetJWKS(ctx)
	if err != nil {
		// No legacy JWKS yet, this is truly first initialisation
		// Not the best place to put this, but we need to know
		// if there's an old JWKS.
		zapctx.Debug(ctx, "no legacy JWKS found, starting fresh")
		meta.RotationInterval = defaultRotationInterval
		meta.GracePeriod = defaultGracePeriod
		meta.MaxTokenLifetime = defaultMaxTokenLifetime
		if err := generateActiveKey(meta, time.Now()); err != nil {
			return errors.E(fmt.Errorf("generate initial active key: %w", err))
		}
		return nil
	}

	// Read legacy private key
	legacyPrivateKey, err := credStore.GetJWKSPrivateKey(ctx)
	if err != nil {
		return errors.E(fmt.Errorf("get legacy private key: %w", err))
	}

	// Extract KID from the legacy JWKS
	keys := legacyJWKS.Keys(ctx)
	if !keys.Next(ctx) {
		return errors.E("legacy JWKS has no keys")
	}

	legacyKey := keys.Pair().Value.(jwk.Key)

	kid, ok := legacyKey.Get(jwk.KeyIDKey)
	if !ok {
		kid = uuid.New().String() // Generate KID if not present
	}
	kidStr := fmt.Sprintf("%v", kid)

	// Convert legacy key to PublicJWK format using JSON marshalling
	buf, err := json.Marshal(legacyKey)
	if err != nil {
		return errors.E(fmt.Errorf("marshal legacy public key: %w", err))
	}

	// Create KeyInfo from legacy key
	now := time.Now()
	keyInfo := vault.KeyInfo{
		KID:         kidStr,
		PrivateKey:  string(legacyPrivateKey),
		PublicJWK:   string(buf),
		ActivatedAt: &now, // Mark as activated now
	}

	meta.Keys = []vault.KeyInfo{keyInfo}
	meta.CurrentActiveKID = kidStr
	meta.RotationInterval = defaultRotationInterval
	meta.GracePeriod = defaultGracePeriod
	meta.MaxTokenLifetime = defaultMaxTokenLifetime

	zapctx.Debug(ctx, "migrated legacy key to metadata",
		zap.String("kid", kidStr))

	return nil
}

// metadataInitialised checks if metadata has been populated (has an active key).
func metadataInitialised(meta *vault.KeyMetadata) bool {
	// If this is first pass, we may have a pregenerated key, but it isn't set to active yet.
	return meta.CurrentActiveKID != "" || len(meta.Keys) > 0
}

// retireExpiredKeys removes keys that are fully expired (past MaxTokenLifetime + GracePeriod).
// These keys can no longer be used for signing or verification.
func retireExpiredKeys(meta *vault.KeyMetadata, now time.Time) {
	var activeKeys []vault.KeyInfo

	for _, k := range meta.Keys {
		// We know it is an active key.
		if k.ActivatedAt != nil {

			// Expiry is when it was activated + a max jwt + grace.
			expiry := k.ActivatedAt.Add(meta.MaxTokenLifetime + meta.GracePeriod)

			// At this point we know it is definitely safe to remove, so skip over it and
			// exclude it from this metadata update.
			if now.After(expiry) {
				zapctx.Debug(
					context.Background(),
					"removing fully expired key",
					zap.String("kid", k.KID),
				)
				continue
			}
		}

		// These keys are active, or preactive, and safe to keep for now.
		activeKeys = append(activeKeys, k)
	}

	meta.Keys = activeKeys
}

// promotePreActiveKey finds a pre-active key whose ActivatedAt <= now and makes it active.
// Only one key is promoted per call.
//
// These keys are generated in a later step under generatePreActiveKey after a needsRotation check.
func promotePreActiveKey(meta *vault.KeyMetadata, now time.Time) {
	for i := range meta.Keys {
		k := &meta.Keys[i]

		// Key is acive.
		if k.KID == meta.CurrentActiveKID {
			continue
		}

		// Skip if not yet scheduled for activation.
		// k.ActivatedAt.After includes the grace period, so we won't activate it until
		// the grace has arrived.
		if k.ActivatedAt == nil || k.ActivatedAt.After(now) {
			continue
		}

		meta.CurrentActiveKID = k.KID

		zapctx.Debug(
			context.Background(),
			"promoting pre-active key to active",
			zap.String("kid", k.KID),
		)

		break
	}
}

// needsRotation checks if the current active key is past its rotation deadline.
func needsRotation(meta *vault.KeyMetadata, now time.Time) bool {
	// If a pre-active key is already queued for future activation, do not create another.
	// This should only happen during initialisation.
	if hasPendingPreActiveKey(meta, now) {
		return false
	}

	// No active key yet: only rotate if there are no keys at all.
	// This avoids generating many pre-active keys while waiting for grace period.
	if meta.CurrentActiveKID == "" {
		return len(meta.Keys) == 0
	}

	// Find the active key and check if it's past the rotation deadline.
	for _, k := range meta.Keys {
		if k.KID == meta.CurrentActiveKID && k.ActivatedAt != nil {
			rotationDeadline := k.ActivatedAt.Add(meta.RotationInterval)
			return now.After(rotationDeadline)
		}
	}

	// No active key found, so we should rotate to create one.
	return true
}

// hasPendingPreActiveKey checks if there is a non active key with an ActivatedAt in the future.
func hasPendingPreActiveKey(meta *vault.KeyMetadata, now time.Time) bool {
	for _, k := range meta.Keys {
		if k.KID == meta.CurrentActiveKID {
			continue
		}
		if k.ActivatedAt != nil && k.ActivatedAt.After(now) {
			return true
		}
	}
	return false
}

// generatePreActiveKey creates a new key with ActivatedAt = now + GracePeriod.
func generatePreActiveKey(meta *vault.KeyMetadata, now time.Time) error {
	activationTime := now.Add(meta.GracePeriod)
	newKey, err := generateJWKvv2(activationTime)
	if err != nil {
		return errors.E(err)
	}

	meta.Keys = append(meta.Keys, newKey)

	zapctx.Debug(
		context.Background(),
		"generated new pre-active key",
		zap.String("kid", newKey.KID),
		zap.Time("activation_time", activationTime),
	)

	return nil
}

// generateActiveKey creates and sets a key as the current active key immediately.
// This is for first time initialisation.
func generateActiveKey(meta *vault.KeyMetadata, now time.Time) error {
	newKey, err := generateJWKvv2(now)
	if err != nil {
		return errors.E(err)
	}

	meta.Keys = append(meta.Keys, newKey)
	meta.CurrentActiveKID = newKey.KID

	zapctx.Debug(
		context.Background(),
		"generated new active key",
		zap.String("kid", newKey.KID),
		zap.Time("activation_time", now),
	)

	return nil
}

func generateJWKvv2(activationTime time.Time) (vault.KeyInfo, error) {
	// Generate RSA key
	priv, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		return vault.KeyInfo{}, errors.E(err)
	}

	privPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(priv),
	})

	pubJWK, err := jwk.FromRaw(priv.PublicKey)
	if err != nil {
		return vault.KeyInfo{}, errors.E(err)
	}

	kid := uuid.New().String()
	if err := pubJWK.Set(jwk.KeyIDKey, kid); err != nil {
		return vault.KeyInfo{}, errors.E(err)
	}
	if err := pubJWK.Set(jwk.KeyUsageKey, "sig"); err != nil {
		return vault.KeyInfo{}, errors.E(err)
	}
	if err := pubJWK.Set(jwk.AlgorithmKey, jwa.RS256); err != nil {
		return vault.KeyInfo{}, errors.E(err)
	}

	pubJWKJSON, err := json.Marshal(pubJWK)
	if err != nil {
		return vault.KeyInfo{}, errors.E(err)
	}

	newKey := vault.KeyInfo{
		KID:         kid,
		PrivateKey:  string(privPEM),
		PublicJWK:   string(pubJWKJSON),
		ActivatedAt: &activationTime,
	}

	return newKey, nil
}

// buildPublishableJWKS constructs a jwk.Set from metadata, including only publishable keys.
func buildPublishableJWKS(meta *vault.KeyMetadata, now time.Time) (jwk.Set, error) {
	ks := jwk.NewSet()

	for _, k := range meta.Keys {
		if !isKeyStillPublishable(&k, meta, now) {
			continue
		}

		key, err := jwk.ParseKey([]byte(k.PublicJWK))
		if err != nil {
			zapctx.Warn(context.Background(), "failed to parse public key",
				zap.String("kid", k.KID),
				zap.Error(err))
			continue
		}

		if err := ks.AddKey(key); err != nil {
			zapctx.Warn(context.Background(), "failed to add key to JWKS",
				zap.String("kid", k.KID),
				zap.Error(err))
			continue
		}
	}

	return ks, nil
}

// isKeyStillPublishable checks if a key should be included in our JWKS endpoint response.
func isKeyStillPublishable(k *vault.KeyInfo, meta *vault.KeyMetadata, now time.Time) bool {
	if k.ActivatedAt == nil {
		// Shouldn't happen.
		return true
	}

	// For activated keys, check if still within the safe window
	expiry := k.ActivatedAt.Add(meta.MaxTokenLifetime + meta.GracePeriod)
	return now.Before(expiry)
}

func getActiveJWKSigningKey(ctx context.Context, meta *vault.KeyMetadata) ([]byte, error) {
	for _, k := range meta.Keys {
		if k.KID == meta.CurrentActiveKID {
			return []byte(k.PrivateKey), nil
		}
	}
	return nil, errors.E("no active key found")
}
