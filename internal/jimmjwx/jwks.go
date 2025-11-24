// Copyright 2025 Canonical.

package jimmjwx

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/juju/zaputil/zapctx"
	"github.com/lestrrat-go/jwx/v2/jwa"
	"github.com/lestrrat-go/jwx/v2/jwk"
	"go.uber.org/zap"

	"github.com/canonical/jimm/v3/internal/errors"
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

// StartJWKSRotator starts a simple routine which checks the vaults TTL for the JWKS on a ticker.C.
// It is expected that this routine will be cleaned up alongside other background services sharing
// the same cancellable context.
//
// TODO(ale8k)[possibly?]:
// For now, there's a single key, and this is probably OK. But possibly extend
// this to contain many at some point differentiated by KIDs.
//
// We also currently don't use x5c and x5t for validation and expect users
// to use e and n for validation.
// https://stackoverflow.com/questions/61395261/how-to-validate-signature-of-jwt-from-jwks-without-x5c
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
