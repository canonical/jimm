package jimmjwx

import (
	"context"
	"net/http"

	"github.com/lestrrat-go/jwx/v2/jwk"
)

// JWTService manages the creation of JWTs that are inteded to be issued
// by JIMM.
type JWTService struct {
	jwksService *JWKSService
}

// NewJWTService returns a new JWT service for handling JIMMs JWTs.
func NewJWTService(jwksService *JWKSService) *JWTService {
	return &JWTService{jwksService: jwksService}
}

// RegisterJWKSCache registers a cache to refresh the public key persisted by JIMM's
// JWKSService. It calls JIMM's JWKSService endpoint the same as any other ordinary
// client would.
func (j *JWTService) RegisterJWKSCache(ctx context.Context, wellknownHost string, client *http.Client) {
	cache := jwk.NewCache(ctx)

	_ = cache.Register("https://"+wellknownHost+"/.well-known/jwks.json", jwk.WithHTTPClient(client))
	if _, err := cache.Refresh(ctx, "https://"+wellknownHost+"/.well-known/jwks.json"); err != nil {
		// url is not a valid JWKS
		panic(err)
	}
}
