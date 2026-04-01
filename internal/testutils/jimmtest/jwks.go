// Copyright 2026 Canonical.

package jimmtest

import (
	"encoding/json"
	"os"
	"path/filepath"

	qt "github.com/frankban/quicktest"

	"github.com/canonical/jimm/v3/internal/jimmjwx"
)

// StaticJWKSServiceParams returns a static JWKS configuration for tests.
func StaticJWKSServiceParams(c *qt.C) (jimmjwx.JWKSServiceParams, error) {
	c.Helper()
	set, privateKey, err := jwkSetFromPrivateKeyFile()
	if err != nil {
		return jimmjwx.JWKSServiceParams{}, err
	}
	rawJWKS, err := json.Marshal(set)
	if err != nil {
		return jimmjwx.JWKSServiceParams{}, err
	}
	dir := c.TempDir()
	jwksPath := filepath.Join(dir, "jwks.json")
	privateKeyPath := filepath.Join(dir, "jwks_private_key.pem")
	if err := os.WriteFile(jwksPath, rawJWKS, 0o600); err != nil {
		return jimmjwx.JWKSServiceParams{}, err
	}
	if err := os.WriteFile(privateKeyPath, privateKey, 0o600); err != nil {
		return jimmjwx.JWKSServiceParams{}, err
	}
	return jimmjwx.JWKSServiceParams{
		JWKSPath:       jwksPath,
		PrivateKeyPath: privateKeyPath,
	}, nil
}

// NewStaticJWKSService returns a deterministic JWKS service for tests.
func NewStaticJWKSService(c *qt.C) (*jimmjwx.JWKSService, error) {
	params, err := StaticJWKSServiceParams(c)
	if err != nil {
		return nil, err
	}
	service, err := jimmjwx.NewJWKSService(c.Context(), params)
	if err != nil {
		return nil, err
	}
	return service, nil
}
