// Copyright 2025 Canonical.

package jimmjwx

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/juju/zaputil/zapctx"
	"github.com/lestrrat-go/jwx/v2/jwa"
	"github.com/lestrrat-go/jwx/v2/jwk"
	"github.com/lestrrat-go/jwx/v2/jwt"
	"go.uber.org/zap"

	"github.com/canonical/jimm/v3/internal/errors"
)

const (
	accessClaim = "access"
)

type JWTServiceParams struct {
	Host   string
	Expiry time.Duration
	JWKS   JwksGetter
}

// JwksGetter provides a Get method to fetch the JWK set.
type JwksGetter interface {
	Get(ctx context.Context) (jwk.Set, error)
	SigningKey(ctx context.Context) (jwk.Key, error)
}

// JWTService manages the creation of JWTs that are intended to be issued
// by JIMM.
type JWTService struct {
	JWTServiceParams
}

// JWTParams are the necessary params to issue a ready-to-go JWT targeted
// at a Juju controller.
type JWTParams struct {
	// Controller is the "aud" of the JWT
	Controller string
	// User is the "sub" of the JWT
	User string
	// Access is a claim of key/values denoting what the user wishes to access
	// stored in a claim called "access".
	Access map[string]string
	// ExtraClaims contain any extra claims that should be added to the JWT.
	// "access" is a reserved claim and will cause an error if used.
	ExtraClaims map[string]any
	// Expiry is the duration after which the JWT will expire.
	// If not set, it will default to the configured default expiry.
	// If set, it will override the JWTServiceParams.Expiry.
	Expiry time.Duration
}

// NewJWTService returns a new JWT service for handling JIMMs JWTs.
func NewJWTService(p JWTServiceParams) *JWTService {
	return &JWTService{JWTServiceParams: p}
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
	jti, err := j.generateJTI()
	if err != nil {
		return nil, err
	}

	zapctx.Debug(ctx, "issuing a new JWT", zap.Any("params", params))

	if j.JWKS == nil {
		return nil, errors.New("missing signing key provider")
	}

	signingKey, err := j.JWKS.SigningKey(ctx)
	if err != nil {
		return nil, err
	}
	if signingKey == nil {
		return nil, errors.New("missing signing key")
	}

	expiry := j.Expiry
	if params.Expiry != time.Duration(0) {
		expiry = params.Expiry
	}

	builder := jwt.NewBuilder().
		Audience([]string{params.Controller}).
		Subject(params.User).
		Issuer(j.Host).
		JwtID(jti).
		Claim(accessClaim, params.Access).
		Expiration(time.Now().Add(expiry))

	for k, v := range params.ExtraClaims {
		if k == accessClaim {
			return nil, errors.New("access is a reserved claim")
		}
		builder = builder.Claim(k, v)
	}

	token, err := builder.Build()
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
func (j *JWTService) generateJTI() (string, error) {
	id, err := uuid.NewRandom()
	if err != nil {
		return "", err
	}
	return id.String(), nil
}
