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

// StartJWKSRotator starts a simple routine which checks the vaults TTL for the JWKS on a ticker
// if the key set is within 1 day of rotation required time, it will rotate the keys.
//
// It closure that may be called to stop the rotation ticker.
//
// TODO(ale8k)[possibly?]:
// For now, there's a single key, and this is probably OK. But possibly extend
// this to contain many at some point differentiated by KIDs.
//
// We also currently don't use x5c and x5t for validation and expect users
// to use e and n for validation.
// https://stackoverflow.com/questions/61395261/how-to-validate-signature-of-jwt-from-jwks-without-x5c
func (jwks *JWKSService) StartJWKSRotator(ctx context.Context, checkRotateRequired *time.Ticker, initialRotateRequiredTime time.Time) error {
	const op = errors.Op("vault.StartJWKSRotator")

	credStore := jwks.credentialStore

	putJwks := func() {
		set, key, err := jwks.generateJWK(ctx)
		if err != nil {
			zapctx.Error(
				ctx,
				"security failure",
				zap.Any("op", op),
				zap.String("security-failure", "failed to generate jwks"),
				zap.Error(err),
			)
			return
		}

		err = credStore.PutJWKS(ctx, set)
		if err != nil {
			zapctx.Error(
				ctx,
				"security failure",
				zap.Any("op", op),
				zap.String("security-failure", "failed to put JWKS"),
			)
			return
		}

		err = credStore.PutJWKSPrivateKey(ctx, key)
		if err != nil {
			zapctx.Error(
				ctx,
				"security failure",
				zap.Any("op", op),
				zap.String("security-failure", "failed to put JWKS private key"),
				zap.Error(err),
			)
			return
		}

		err = credStore.PutJWKSExpiry(ctx, initialRotateRequiredTime)
		if err != nil {
			zapctx.Error(
				ctx,
				"security failure",
				zap.Any("op", op),
				zap.String("security-failure", "failed to put JWKS expiry time key"),
				zap.Error(err),
			)
			return
		}

		zapctx.Debug(ctx, "set a new JWKS")

	}

	go func() {
		for {
			select {
			case <-checkRotateRequired.C:
				expires, err := credStore.GetJWKSExpiry(ctx)

				if err != nil {
					zapctx.Debug(ctx, "failed to get expiry", zap.Error(err))
					putJwks()
				}
				// If we recieve the expiry, we make a simple check 3 months ahead.
				now := time.Now().UTC()
				if now.After(expires) {
					putJwks()
				}
			case <-ctx.Done():
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
func (s *JWKSService) generateJWK(ctx context.Context) (jwk.Set, []byte, error) {
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
