package jimmjwx

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"net/http"

	"github.com/CanonicalLtd/jimm/internal/jimm/credentials"
	"github.com/google/uuid"
	"github.com/lestrrat-go/jwx/v2/jwa"
	"github.com/lestrrat-go/jwx/v2/jwk"
	"github.com/lestrrat-go/jwx/v2/jwt"
)

// JWTService manages the creation of JWTs that are inteded to be issued
// by JIMM.
type JWTService struct {
	jwksService *JWKSService
	Cache       *jwk.Cache
	host        string
	store       credentials.CredentialStore
}

type JWTParams struct {
	// Controller is the "aud" of the JWT
	Controller string
	// User is the "sub" of the JWT
	User string
	// Access is a claim of key/values denoting what the user wishes to access
	Access map[string]any
}

// NewJWTService returns a new JWT service for handling JIMMs JWTs.
func NewJWTService(jwksService *JWKSService, host string, store credentials.CredentialStore) *JWTService {
	return &JWTService{jwksService: jwksService, host: host, store: store}
}

// RegisterJWKSCache registers a cache to refresh the public key persisted by JIMM's
// JWKSService. It calls JIMM's JWKSService endpoint the same as any other ordinary
// client would.
func (j *JWTService) RegisterJWKSCache(ctx context.Context, client *http.Client) {
	j.Cache = jwk.NewCache(ctx)

	_ = j.Cache.Register("https://"+j.host+"/.well-known/jwks.json", jwk.WithHTTPClient(client))
	if _, err := j.Cache.Refresh(ctx, "https://"+j.host+"/.well-known/jwks.json"); err != nil {
		// url is not a valid JWKS
		panic(err)
	}
}

// NewJWT creates a new JWT to be represent a users access within a controller.
//
//   - The Issuer is resolved from this function.
//   - The JWT ID should be cached and validated on each call, where the client verifies it has not been used before.
//     Once the JWT has expired for said ID, the client can clean up their blacklist.
func (j *JWTService) NewJWT(ctx context.Context, params JWTParams) (jwt.Token, error) {
	if jti, err := j.generateJTI(ctx); err != nil {
		return nil, err
	} else {

		pkeyPem, err := j.store.GetJWKSPrivateKey(ctx)
		if err != nil {
		}

		block, _ := pem.Decode([]byte(pkeyPem))

		pkeyDecoded, err := x509.ParsePKCS1PrivateKey(block.Bytes)
		if err != nil {
		}

		signingKey, err := jwk.FromRaw(pkeyDecoded)
		if err != nil {
		}

		signingKey.Set(jwk.AlgorithmKey, jwa.RS256)
		signingKey.Set(jwk.KeyIDKey, <set kid here>)// TODO set kid from cached jwks alex

		token, err := jwt.NewBuilder().
			Audience([]string{params.Controller}).
			Subject(params.User).
			Issuer("jimm"). // Resolve current host name from os.Getenv alex
			JwtID(jti).
			Claim("access", params.Access).
			Build()

		// jwt.Sign(
		// 	token,
		// 	jwt.WithKey(
		// 		jwa.RS256,

		// 		)
		// 	)
		return token, err
	}
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
