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

func (j *JWTService) RegisterJWKSCache(ctx context.Context, wellknownHost string) {
	cache := jwk.NewCache(ctx)
	// The default config here is perfectly fine for this use-case and we
	// only wish to append TLS (given that it is provided and we require it)
	transport := http.DefaultTransport.(*http.Transport).Clone()

	client := &http.Client{
		Transport: transport,
	}

	_ = cache.Register(wellknownHost+"/.well-known/jwks.json", jwk.WithHTTPClient(client))
	if _, err := cache.Refresh(ctx, wellknownHost+"/.well-known/jwks.json"); err != nil {
		// url is not a valid JWKS
		panic(err)
	}
}
