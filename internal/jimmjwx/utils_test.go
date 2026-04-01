// Copyright 2025 Canonical.

package jimmjwx

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"os"
	"path/filepath"
	"time"

	qt "github.com/frankban/quicktest"
	"github.com/google/uuid"
	"github.com/lestrrat-go/jwx/v2/jwa"
	"github.com/lestrrat-go/jwx/v2/jwk"
)

// generateJWK generates a fresh RSA keypair and corresponding JWKS for tests.
//
// It will return a jwk.Set containing the public key
// and a PEM encoded private key for JWT signing.
func generateJWK(c *qt.C) (jwk.Set, []byte) {
	c.Helper()

	keySet, err := rsa.GenerateKey(rand.Reader, 2048)
	c.Assert(err, qt.IsNil)

	privateKeyPEM := pem.EncodeToMemory(
		&pem.Block{
			Type:  "RSA PRIVATE KEY",
			Bytes: x509.MarshalPKCS1PrivateKey(keySet),
		},
	)

	kid, err := uuid.NewRandom()
	c.Assert(err, qt.IsNil)

	jwks, err := jwk.FromRaw(keySet.PublicKey)
	c.Assert(err, qt.IsNil)
	err = jwks.Set(jwk.KeyIDKey, kid.String())
	c.Assert(err, qt.IsNil)

	err = jwks.Set(jwk.KeyUsageKey, "sig")
	c.Assert(err, qt.IsNil)

	err = jwks.Set(jwk.AlgorithmKey, jwa.RS256)
	c.Assert(err, qt.IsNil)

	ks := jwk.NewSet()
	err = ks.AddKey(jwks)
	c.Assert(err, qt.IsNil)

	return ks, privateKeyPEM
}

func newJWKSServiceParams(c *qt.C) (JWKSServiceParams, jwk.Set, []byte) {
	c.Helper()
	set, privateKey := generateJWK(c)
	rawJWKS, err := json.Marshal(set)
	c.Assert(err, qt.IsNil)
	dir := c.TempDir()
	jwksPath := filepath.Join(dir, "jwks.json")
	privateKeyPath := filepath.Join(dir, "jwks_private_key.pem")
	err = os.WriteFile(jwksPath, rawJWKS, 0o600)
	c.Assert(err, qt.IsNil)
	err = os.WriteFile(privateKeyPath, privateKey, 0o600)
	c.Assert(err, qt.IsNil)
	return JWKSServiceParams{
		JWKSPath:       jwksPath,
		PrivateKeyPath: privateKeyPath,
	}, set, privateKey
}

func newJWKSService(c *qt.C) (*JWKSService, jwk.Set) {
	c.Helper()
	params, set, _ := newJWKSServiceParams(c)
	service, err := NewJWKSService(c.Context(), params)
	c.Assert(err, qt.IsNil)
	return service, set
}

func newJWTService(c *qt.C, expiry time.Duration) (*JWTService, jwk.Set) {
	c.Helper()
	service, set := newJWKSService(c)
	return NewJWTService(JWTServiceParams{
		Host:   "host",
		Expiry: expiry,
		JWKS:   service,
	}), set
}
