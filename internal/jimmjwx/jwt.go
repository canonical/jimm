// Copyright 2023 Canonical Ltd.

package jimmjwx

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/canonical/jimm/internal/errors"
	"github.com/canonical/jimm/internal/jimm/credentials"
	"github.com/google/uuid"
	"github.com/juju/clock"
	"github.com/juju/retry"
	"github.com/juju/zaputil/zapctx"
	"github.com/lestrrat-go/jwx/v2/jwa"
	"github.com/lestrrat-go/jwx/v2/jwk"
	"github.com/lestrrat-go/jwx/v2/jwt"
	"go.uber.org/zap"
)

// JWTService manages the creation of JWTs that are inteded to be issued
// by JIMM.
type JWTService struct {
	Cache *jwk.Cache
	host  string
	https bool
	store credentials.CredentialStore
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
func NewJWTService(host string, store credentials.CredentialStore, https bool) *JWTService {
	return &JWTService{host: host, store: store, https: https}
}

// RegisterJWKSCache registers a cache to refresh the public key persisted by JIMM's
// JWKSService. It calls JIMM's JWKSService endpoint the same as any other ordinary
// client would.
func (j *JWTService) RegisterJWKSCache(ctx context.Context, client *http.Client) {
	j.Cache = jwk.NewCache(ctx)

	_ = j.Cache.Register(j.getJWKSEndpoint(j.https), jwk.WithHTTPClient(client))

	err := retry.Call(retry.CallArgs{
		Func: func() error {
			zapctx.Info(ctx, "cache refresh")
			select {
			case <-ctx.Done():
				return nil
			default:
			}
			if _, err := j.Cache.Refresh(ctx, j.getJWKSEndpoint(j.https)); err != nil {
				zapctx.Debug(ctx, "Refresh error", zap.Error(err), zap.String("URL", j.getJWKSEndpoint(j.https)))
				return err
			}
			return nil
		},
		Attempts: 10,
		Delay:    2 * time.Second,
		Clock:    clock.WallClock,
		Stop:     ctx.Done(),
	})
	select {
	case <-ctx.Done():
		zapctx.Info(ctx, "context cancelled stopping jwks registration gracefully", zap.Error(err))
	default:
		if err != nil {
			panic(err.Error())
		}
	}
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
	if j == nil || j.Cache == nil {
		zapctx.Debug(ctx, "JwtService struct value", zap.String("JwtService", fmt.Sprintf("%+v", j)))
		return nil, errors.E("nil pointer in JWT service")
	}

	jwkSet, err := j.Cache.Get(ctx, j.getJWKSEndpoint(j.https))
	if err != nil {
		return nil, err
	}

	pubKey, ok := jwkSet.Key(jwkSet.Len() - 1)
	if !ok {
		zapctx.Error(ctx, "no jwk found")
		return nil, errors.E("no jwk found")
	}
	pkeyPem, err := j.store.GetJWKSPrivateKey(ctx)
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

	expiryDuration, err := time.ParseDuration(os.Getenv("JIMM_JWT_EXPIRY"))
	if err != nil {
		zapctx.Error(ctx, "failed to get JIMM_JWT_EXPIRY environment variable", zap.Error(err))
		return nil, err
	}

	signingKey.Set(jwk.AlgorithmKey, jwa.RS256)
	signingKey.Set(jwk.KeyIDKey, pubKey.KeyID())

	token, err := jwt.NewBuilder().
		Audience([]string{params.Controller}).
		Subject(params.User).
		Issuer(os.Getenv("JIMM_DNS_NAME")).
		JwtID(jti).
		Claim("access", params.Access).
		Expiration(time.Now().Add(expiryDuration)).
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

func (j *JWTService) getJWKSEndpoint(secure bool) string {
	scheme := "https://"
	if !secure {
		scheme = "http://"
	}
	return scheme + j.host + "/.well-known/jwks.json"
}
