// Copyright 2023 canonical.

package credentials

import (
	"context"
	"time"

	"github.com/juju/names/v5"
	"github.com/lestrrat-go/jwx/v2/jwk"
)

// A CredentialStore is a store for the attributes of:
//   - Cloud credentials
//   - Controller credentials
//   - JWK Set
//   - JWK expiry
//   - JWK private key
type CredentialStore interface {
	// Get retrieves the stored attributes of a cloud credential.
	Get(context.Context, names.CloudCredentialTag) (map[string]string, error)

	// Put stores the attributes of a cloud credential.
	Put(context.Context, names.CloudCredentialTag, map[string]string) error

	// GetControllerCredentials retrieves the credentials for the given controller from a vault
	// service.
	GetControllerCredentials(ctx context.Context, controllerName string) (string, string, error)

	// PutControllerCredentials stores the controller credentials in a vault
	// service.
	PutControllerCredentials(ctx context.Context, controllerName string, username string, password string) error

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

	// GetOAuthKey returns the current HS256 (symmetric) key used to sign OAuth session tokens.
	GetOAuthKey(ctx context.Context) ([]byte, error)

	// PutOAuthKey puts a HS256 (symmetric) key into the credentials store for signing OAuth session tokens.
	PutOAuthKey(ctx context.Context, raw []byte) error
}
