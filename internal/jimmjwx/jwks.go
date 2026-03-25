// Copyright 2025 Canonical.

package jimmjwx

import (
	"context"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"strconv"

	"github.com/lestrrat-go/jwx/v2/jwa"
	"github.com/lestrrat-go/jwx/v2/jwk"

	"github.com/canonical/jimm/v3/internal/errors"
)

type JWKSServiceParams struct {
	JWKS          string
	PrivateKeyPEM string
	CacheMaxAge   string
}

// JWKSService serves operator-managed JWKS material for JIMM.
type JWKSService struct {
	set         jwk.Set
	signingKey  jwk.Key
	cacheMaxAge int64
}

// NewJWKSService parses and validates the operator-managed JWKS configuration.
func NewJWKSService(p JWKSServiceParams) (*JWKSService, error) {
	if p.JWKS == "" {
		return nil, errors.New("missing jwks")
	}
	if p.PrivateKeyPEM == "" {
		return nil, errors.New("missing jwks private key")
	}
	if p.CacheMaxAge == "" {
		return nil, errors.New("missing jwks cache max age")
	}

	cacheMaxAge, err := strconv.ParseInt(p.CacheMaxAge, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("parse jwks cache max age: %w", err)
	}
	if cacheMaxAge <= 0 {
		return nil, errors.New("jwks cache max age must be greater than 0")
	}

	set, err := jwk.ParseString(p.JWKS)
	if err != nil {
		return nil, fmt.Errorf("parse jwks: %w", err)
	}
	if set.Len() == 0 {
		return nil, errors.New("jwks must contain at least one key")
	}

	privateKey, err := parseRSAPrivateKey([]byte(p.PrivateKeyPEM))
	if err != nil {
		return nil, err
	}

	signingKey, err := jwk.FromRaw(privateKey)
	if err != nil {
		return nil, fmt.Errorf("create jwks signing key: %w", err)
	}
	if err := signingKey.Set(jwk.AlgorithmKey, jwa.RS256); err != nil {
		return nil, err
	}
	if err := signingKey.Set(jwk.KeyUsageKey, "sig"); err != nil {
		return nil, err
	}

	publicKey, err := matchingPublicKey(set, &privateKey.PublicKey)
	if err != nil {
		return nil, err
	}
	if publicKey.KeyID() != "" {
		if err := signingKey.Set(jwk.KeyIDKey, publicKey.KeyID()); err != nil {
			return nil, err
		}
	}

	return &JWKSService{
		set:         set,
		signingKey:  signingKey,
		cacheMaxAge: cacheMaxAge,
	}, nil
}

func (jwks *JWKSService) Get(_ context.Context) (jwk.Set, error) {
	if jwks == nil {
		return nil, errors.New("missing jwks service")
	}
	return jwks.set, nil
}

func (jwks *JWKSService) CacheMaxAge() int64 {
	if jwks == nil {
		return 0
	}
	return jwks.cacheMaxAge
}

func (jwks *JWKSService) SigningKey() jwk.Key {
	if jwks == nil {
		return nil
	}
	return jwks.signingKey
}

func parseRSAPrivateKey(privateKeyPEM []byte) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode(privateKeyPEM)
	if block == nil {
		return nil, errors.New("failed to decode jwks private key")
	}

	privateKey, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err == nil {
		return privateKey, nil
	}

	pkcs8Key, pkcs8Err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if pkcs8Err != nil {
		return nil, fmt.Errorf("parse jwks private key: %w", err)
	}

	rsaKey, ok := pkcs8Key.(*rsa.PrivateKey)
	if !ok {
		return nil, errors.New("jwks private key is not rsa")
	}
	return rsaKey, nil
}

func matchingPublicKey(set jwk.Set, signingPublicKey *rsa.PublicKey) (jwk.Key, error) {
	for i := 0; i < set.Len(); i++ {
		key, ok := set.Key(i)
		if !ok {
			continue
		}

		var publicKey rsa.PublicKey
		if err := key.Raw(&publicKey); err != nil {
			continue
		}
		if publicKey.E == signingPublicKey.E && publicKey.N.Cmp(signingPublicKey.N) == 0 {
			return key, nil
		}
	}

	return nil, errors.New("jwks does not contain the public key for the provided private key")
}
