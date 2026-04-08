// Copyright 2025 Canonical.

package jimmjwx

import (
	"bytes"
	"context"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/juju/zaputil/zapctx"
	"github.com/lestrrat-go/jwx/v2/jwa"
	"github.com/lestrrat-go/jwx/v2/jwk"
	"go.uber.org/zap"

	"github.com/canonical/jimm/v3/internal/errors"
)

// JWKSService implements JWKSProvider to serve operator-managed JWKS material for JIMM.
type JWKSServiceParams struct {
	JWKSPath       string
	PrivateKeyPath string
}

type cachedJWKS struct {
	set        jwk.Set
	signingKey jwk.Key
}

// jwksRefreshInterval defines how often the JWKS material is refreshed from disk.
// This value should be << expected frequency of updates to the data to ensure changes
// are picked up in a timely manner, but not so low as to cause excessive disk reads.
const jwksRefreshInterval = 5 * time.Minute

// JWKSService serves operator-managed JWKS material for JIMM.
type JWKSService struct {
	mu sync.RWMutex

	jwksPath       string
	privateKeyPath string
	cached         cachedJWKS
}

// NewJWKSService reads and periodically refreshes the JWKS material from the provided paths.
//
// The file paths are expected to be managed by the operator, e.g. a Juju charm or otherwise.
// JIMM reads from the files with a relatively high frequency (every 5 minutes) compared to
// the expected frequency of updates (hours/days) to allow for changes to be picked up in a
// timely manner without requiring a restart of the service.
//
// The provided context is used to manage the lifecycle of the refresh loop; cancelling the
// context will stop the loop and must be done on cleanup.
func NewJWKSService(ctx context.Context, p JWKSServiceParams) (*JWKSService, error) {
	if p.JWKSPath == "" {
		return nil, errors.New("missing jwks path")
	}
	if p.PrivateKeyPath == "" {
		return nil, errors.New("missing jwks private key path")
	}

	material, err := loadJWKS(p.JWKSPath, p.PrivateKeyPath)
	if err != nil {
		return nil, err
	}

	service := &JWKSService{
		jwksPath:       p.JWKSPath,
		privateKeyPath: p.PrivateKeyPath,
		cached:         material,
	}
	go service.refreshLoop(ctx)
	return service, nil
}

// Get returns the JWKS set to be served at /.well-known/jwks.json.
func (jwks *JWKSService) Get(_ context.Context) (jwk.Set, error) {
	jwks.mu.RLock()
	defer jwks.mu.RUnlock()
	if jwks.cached.set == nil {
		return nil, errors.New("missing jwks")
	}
	return jwks.cached.set, nil
}

// SigningKey returns the jwk.Key to be used for signing JWTs.
// This is the private key corresponding to one of the public keys in the JWKS.
func (jwks *JWKSService) SigningKey(_ context.Context) (jwk.Key, error) {
	jwks.mu.RLock()
	defer jwks.mu.RUnlock()
	if jwks.cached.signingKey == nil {
		return nil, errors.New("missing signing key")
	}
	return jwks.cached.signingKey, nil
}

func (jwks *JWKSService) refresh(ctx context.Context) {
	material, err := loadJWKS(jwks.jwksPath, jwks.privateKeyPath)
	if err != nil {
		zapctx.Error(ctx, "failed to refresh jwks, using cached value", zap.Error(err))
		return
	}

	jwks.mu.Lock()
	defer jwks.mu.Unlock()
	jwks.cached = material
}

func (jwks *JWKSService) refreshLoop(ctx context.Context) {
	ticker := time.NewTicker(jwksRefreshInterval)

	for {
		select {
		case <-ticker.C:
			jwks.refresh(ctx)
		case <-ctx.Done():
			zapctx.Info(ctx, "exiting jwks refresh polling")
			return
		}
	}
}

func loadJWKS(jwksPath, privateKeyPath string) (cachedJWKS, error) {
	rawJWKS, err := readRequiredFile(jwksPath, "jwks")
	if err != nil {
		return cachedJWKS{}, err
	}

	set, err := jwk.Parse(rawJWKS)
	if err != nil {
		return cachedJWKS{}, fmt.Errorf("parse jwks: %w", err)
	}
	if set.Len() == 0 {
		return cachedJWKS{}, errors.New("jwks must contain at least one key")
	}

	privateKeyPEM, err := readRequiredFile(privateKeyPath, "jwks private key")
	if err != nil {
		return cachedJWKS{}, err
	}

	privateKey, err := parseRSAPrivateKey(privateKeyPEM)
	if err != nil {
		return cachedJWKS{}, err
	}

	signingKey, err := jwk.FromRaw(privateKey)
	if err != nil {
		return cachedJWKS{}, fmt.Errorf("create jwks signing key: %w", err)
	}
	if err := signingKey.Set(jwk.AlgorithmKey, jwa.RS256); err != nil {
		return cachedJWKS{}, err
	}
	if err := signingKey.Set(jwk.KeyUsageKey, "sig"); err != nil {
		return cachedJWKS{}, err
	}

	publicKey, err := matchingPublicKey(set, &privateKey.PublicKey)
	if err != nil {
		return cachedJWKS{}, err
	}
	if publicKey.KeyID() != "" {
		if err := signingKey.Set(jwk.KeyIDKey, publicKey.KeyID()); err != nil {
			return cachedJWKS{}, err
		}
	}

	return cachedJWKS{
		set:        set,
		signingKey: signingKey,
	}, nil
}

func readRequiredFile(path, name string) ([]byte, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", name, err)
	}
	if len(bytes.TrimSpace(content)) == 0 {
		return nil, errors.New(name + " file is empty")
	}
	return content, nil
}

func parseRSAPrivateKey(privateKeyPEM []byte) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode(privateKeyPEM)
	if block == nil {
		return nil, errors.New("failed to decode jwks private key")
	}

	privateKey, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err == nil {
		return privateKey, nil
	}

	pkcs8Key, pkcs8Err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if pkcs8Err != nil {
		return nil, fmt.Errorf("parse jwks private key: %w", err)
	}

	rsaKey, ok := pkcs8Key.(*rsa.PrivateKey)
	if !ok {
		return nil, errors.New("jwks private key is not rsa")
	}
	return rsaKey, nil
}

func matchingPublicKey(set jwk.Set, signingPublicKey *rsa.PublicKey) (jwk.Key, error) {
	signingKey, err := jwk.PublicKeyOf(signingPublicKey)
	if err != nil {
		return nil, fmt.Errorf("create jwk public key: %w", err)
	}

	for i := 0; i < set.Len(); i++ {
		key, ok := set.Key(i)
		if !ok {
			continue
		}
		if jwk.Equal(key, signingKey) {
			return key, nil
		}
	}

	return nil, errors.New("jwks does not contain the public key for the provided private key")
}
