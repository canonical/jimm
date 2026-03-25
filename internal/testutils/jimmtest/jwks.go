// Copyright 2026 Canonical.

package jimmtest

import (
	"encoding/json"

	"github.com/canonical/jimm/v3/internal/jimmjwx"
)

// StaticJWKSServiceParams returns a static JWKS configuration for tests.
func StaticJWKSServiceParams() (jimmjwx.JWKSServiceParams, error) {
	set, privateKey, err := jwkSetFromPrivateKeyFile()
	if err != nil {
		return jimmjwx.JWKSServiceParams{}, err
	}
	rawJWKS, err := json.Marshal(set)
	if err != nil {
		return jimmjwx.JWKSServiceParams{}, err
	}
	return jimmjwx.JWKSServiceParams{
		JWKS:          string(rawJWKS),
		PrivateKeyPEM: string(privateKey),
		CacheMaxAge:   "600",
	}, nil
}

// NewStaticJWKSService returns a deterministic JWKS service for tests.
func NewStaticJWKSService() (*jimmjwx.JWKSService, error) {
	params, err := StaticJWKSServiceParams()
	if err != nil {
		return nil, err
	}
	return jimmjwx.NewJWKSService(params)
}
