// Copyright 2023 CanonicalLtd.

package jimmjwx

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"time"

	"github.com/google/uuid"
	"github.com/juju/zaputil/zapctx"
	"github.com/lestrrat-go/jwx/v2/jwa"
	"github.com/lestrrat-go/jwx/v2/jwk"
	"go.uber.org/zap"

	"github.com/CanonicalLtd/jimm/internal/errors"
	"github.com/CanonicalLtd/jimm/internal/jimm/credentials"
)

// JWKSService handles the creation, rotation and retrieval of JWKS for JIMM.
// It utilises the underlying credential store currently in effect.
type JWKSService struct {
	credentialStore credentials.CredentialStore
}

// NewJWKSService returns a new JWKS service for handling JIMMs JWKS.
func NewJWKSService(credStore credentials.CredentialStore) *JWKSService {
	return &JWKSService{credentialStore: credStore}
}

func rotateJWKS(ctx context.Context, credStore credentials.CredentialStore, initialExpiryTime time.Time) error {
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
			credStore.CleanupJWKS(ctx)
			return errors.E(err)
		}
	} else {
		// Check it has expired.
		now := time.Now().UTC()
		if now.After(expires) {
			// In theory, an error should not happen anymore as the necessary
			// components exist from the previous failed expiry attempt.
			err = putJwks(time.Now().UTC().AddDate(0, 3, 0))
			if err != nil {
				credStore.CleanupJWKS(ctx)
				return errors.E(err)
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
	const op = errors.Op("vault.StartJWKSRotator")

	credStore := jwks.credentialStore

	// For logging and monitoring purposes, we have the rotator spit errors into
	// this buffered channel ((size * amount) * 2 of errors we are currently aware of and doubling it to prevent blocks)
	errorChan := make(chan error, 8)

	if err := rotateJWKS(ctx, credStore, initialRotateRequiredTime); err != nil {
		return errors.E(op, err)
	}

	// The rotation method is as follows, if an expiry is not present, we know
	// this is the first attempt to set the initial JWKS (or it may be subsequent from erroneous attempts).
	// As the next attempt comes around, it is a simple check if the times is after the current.
	//
	// In this case we generate a new set, which should expire in 3 months.
	go func() {
		defer close(errorChan)
		for {
			select {
			case <-checkRotateRequired:
				if err := rotateJWKS(ctx, credStore, initialRotateRequiredTime); err != nil {
					errorChan <- err
				}

			case <-ctx.Done():
				zapctx.Debug(ctx, "Shutdown for JWKS rotator complete.")
				return
			}
		}
	}()

	// If for any reason the rotator has an error, we simply receive the error
	// in another routine dedicated to logging said errors.
	go func(errChan <-chan error) {
		for err := range errChan {
			zapctx.Error(
				ctx,
				"security failure",
				zap.Any("op", op),
				zap.NamedError("jwks-error", err),
			)
		}
	}(errorChan)

	return nil
}

// generateJWKS generates a new set of JWK using RSA256[4096]
//
// It will return a jwk.Set containing the public key
// and a PEM encoded private key for JWT signing.
func generateJWK(ctx context.Context) (jwk.Set, []byte, error) {
	const op = errors.Op("vault.generateJWKS")

	// Due to the sensitivity of controllers, it is best we allow a larger encryption bit size
	// and accept any negligible wire cost.
	keySet, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		return nil, nil, errors.E(op, err)
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
		return nil, nil, errors.E(op, err)
	}

	jwks, err := jwk.FromRaw(keySet.PublicKey)
	if err != nil {
		return nil, nil, errors.E(op, err)
	}
	err = jwks.Set(jwk.KeyIDKey, kid.String())
	if err != nil {
		return nil, nil, errors.E(op, err)
	}

	err = jwks.Set(jwk.KeyUsageKey, "sig") // Couldn't find const for this...
	if err != nil {
		return nil, nil, errors.E(op, err)
	}

	err = jwks.Set(jwk.AlgorithmKey, jwa.RS256)
	if err != nil {
		return nil, nil, errors.E(op, err)
	}

	ks := jwk.NewSet()
	err = ks.AddKey(jwks)
	if err != nil {
		return nil, nil, errors.E(op, err)
	}

	return ks, privateKeyPEM, nil
}
