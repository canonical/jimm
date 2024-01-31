// Copyright 2023 Canonical Ltd.

package jimmjwx

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"time"

	"github.com/canonical/jimm/internal/errors"
	"github.com/canonical/jimm/internal/jimm/credentials"
	"github.com/google/uuid"
	"github.com/hashicorp/golang-lru/v2/expirable"
	"github.com/juju/zaputil/zapctx"
	"github.com/lestrrat-go/jwx/v2/jwa"
	"github.com/lestrrat-go/jwx/v2/jwk"
	"github.com/lestrrat-go/jwx/v2/jwt"
	"go.uber.org/zap"
)

type JWTServiceParams struct {
	Host   string
	Store  credentials.CredentialStore
	Expiry time.Duration
}

// JwksGetter provides a Get method to fetch the JWK set.
type JwksGetter interface {
	Get(ctx context.Context) (jwk.Set, error)
}

// CredentialCache provides a cache that will periodically fetch the JWK set from a credential store.
type CredentialCache struct {
	credentials.CredentialStore
	c *expirable.LRU[string, jwk.Set]
}

// NewCredentialCache creates a new CredentialCache used for storing and periodically fetching
// a JWK set from the provided credential store.
// Note that the cache duration is configured at 1h, which should be much lower than the
// rotation period of the JWK set.
func NewCredentialCache(credentialStore credentials.CredentialStore) CredentialCache {
	cache := expirable.NewLRU[string, jwk.Set](1, nil, time.Duration(1*time.Hour))
	return CredentialCache{CredentialStore: credentialStore, c: cache}
}

const jwksCacheKey = "jwks"

// Get implements JwksGetter.Get
func (v CredentialCache) Get(ctx context.Context) (jwk.Set, error) {
	if val, ok := v.c.Get(jwksCacheKey); ok {
		return val, nil
	}
	ks, err := v.CredentialStore.GetJWKS(ctx)
	if err != nil {
		return nil, err
	}
	v.c.Add(jwksCacheKey, ks)
	return ks, nil
}

// JWTService manages the creation of JWTs that are intended to be issued
// by JIMM.
type JWTService struct {
	JWTServiceParams
	// JWKS is the JSON Web Key Set containing the public key used for verifying
	// signed JWT tokens.
	JWKS JwksGetter
}

// JWTParams are the necessary params to issue a ready-to-go JWT targeted
// at a Juju controller.
type JWTParams struct {
	// Controller is the "aud" of the JWT
	Controller string
	// User is the "sub" of the JWT
	User string
	// Access is a claim of key/values denoting what the user wishes to access
	Access map[string]string
}

// NewJWTService returns a new JWT service for handling JIMMs JWTs.
func NewJWTService(p JWTServiceParams) *JWTService {
	vaultCache := NewCredentialCache(p.Store)
	return &JWTService{JWTServiceParams: p, JWKS: vaultCache}
}

// NewJWT creates a new JWT to represent a users access within a controller.
//
//   - The Issuer is resolved from this function.
//   - The JWT ID should be cached and validated on each call, where the client verifies it has not been used before.
//     Once the JWT has expired for said ID, the client can clean up their blacklist.
//
// The current usecase of these JWTs is expected that NO session tokens will be generated
// and instead, a new JWT will be issued each time containing the required claims for
// authz.
func (j *JWTService) NewJWT(ctx context.Context, params JWTParams) ([]byte, error) {
	jti, err := j.generateJTI(ctx)
	if err != nil {
		return nil, err
	}

	zapctx.Debug(ctx, "issuing a new JWT", zap.Any("params", params))

	jwkSet, err := j.JWKS.Get(ctx)
	if err != nil {
		return nil, err
	}

	pubKey, ok := jwkSet.Key(jwkSet.Len() - 1)
	if !ok {
		zapctx.Error(ctx, "no jwk found")
		return nil, errors.E("no jwk found")
	}
	pkeyPem, err := j.Store.GetJWKSPrivateKey(ctx)
	if err != nil {
		zapctx.Error(ctx, "failed to retrieve private key", zap.Error(err))
		return nil, err
	}

	block, _ := pem.Decode([]byte(pkeyPem))

	pkeyDecoded, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		zapctx.Error(ctx, "failed to parse private key", zap.Error(err))
		return nil, err
	}

	signingKey, err := jwk.FromRaw(pkeyDecoded)
	if err != nil {
		zapctx.Error(ctx, "failed to create signing key", zap.Error(err))
		return nil, err
	}

	signingKey.Set(jwk.AlgorithmKey, jwa.RS256)
	signingKey.Set(jwk.KeyIDKey, pubKey.KeyID())

	token, err := jwt.NewBuilder().
		Audience([]string{params.Controller}).
		Subject(params.User).
		Issuer(j.Host).
		JwtID(jti).
		Claim("access", params.Access).
		Expiration(time.Now().Add(j.Expiry)).
		Build()
	if err != nil {
		zapctx.Error(ctx, "failed to create token", zap.Error(err))
		return nil, err
	}
	freshToken, err := jwt.Sign(
		token,
		jwt.WithKey(
			jwa.RS256,
			signingKey,
		),
	)
	if err != nil {
		zapctx.Error(ctx, "failed to sign token", zap.Error(err))
		return nil, err
	}
	return freshToken, err
}

// generateJTI uses a V4 UUID, giving a chance of 1 in 17Billion per year.
// This should be good enough (hopefully) for a JWT ID.
func (j *JWTService) generateJTI(ctx context.Context) (string, error) {
	id, err := uuid.NewRandom()
	if err != nil {
		return "", err
	}
	return id.String(), nil
}
