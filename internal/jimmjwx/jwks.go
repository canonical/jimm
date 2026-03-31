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
	"strconv"
	"sync"
	"time"

	"github.com/juju/zaputil/zapctx"
	"github.com/lestrrat-go/jwx/v2/jwa"
	"github.com/lestrrat-go/jwx/v2/jwk"
	"go.uber.org/zap"

	"github.com/canonical/jimm/v3/internal/errors"
)

type JWKSServiceParams struct {
	JWKSPath       string
	PrivateKeyPath string
	CacheMaxAge    string
}

type cachedJWKS struct {
	set        jwk.Set
	signingKey jwk.Key
}

// JWKSService serves operator-managed JWKS material for JIMM.
type JWKSService struct {
	mu        sync.RWMutex
	closeOnce sync.Once

	jwksPath       string
	privateKeyPath string
	cached         cachedJWKS
	cacheMaxAge    int64
	stopCh         chan struct{}
	doneCh         chan struct{}
}

// NewJWKSService parses and validates the operator-managed JWKS configuration.
func NewJWKSService(p JWKSServiceParams) (*JWKSService, error) {
	if p.JWKSPath == "" {
		return nil, errors.New("missing jwks path")
	}
	if p.PrivateKeyPath == "" {
		return nil, errors.New("missing jwks private key path")
	}
	if p.CacheMaxAge == "" {
		return nil, errors.New("missing jwks cache max age")
	}

	cacheMaxAge, err := strconv.ParseInt(p.CacheMaxAge, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("parse jwks cache max age: %w", err)
	}
	if cacheMaxAge <= 0 {
		return nil, errors.New("jwks cache max age must be greater than 0")
	}

	material, err := loadJWKS(p.JWKSPath, p.PrivateKeyPath)
	if err != nil {
		return nil, err
	}

	service := &JWKSService{
		jwksPath:       p.JWKSPath,
		privateKeyPath: p.PrivateKeyPath,
		cached:         material,
		cacheMaxAge:    cacheMaxAge,
		stopCh:         make(chan struct{}),
		doneCh:         make(chan struct{}),
	}
	go service.refreshLoop()
	return service, nil
}

// Get returns the JWKS set to be served at /.well-known/jwks.json.
func (jwks *JWKSService) Get(_ context.Context) (jwk.Set, error) {
	if jwks == nil {
		return nil, errors.New("missing jwks service")
	}
	jwks.mu.RLock()
	defer jwks.mu.RUnlock()
	if jwks.cached.set == nil {
		return nil, errors.New("missing jwks")
	}
	return jwks.cached.set, nil
}

// CacheMaxAge returns the Cache-Control max-age, in seconds, for /.well-known/jwks.json.
func (jwks *JWKSService) CacheMaxAge() int64 {
	if jwks == nil {
		return 0
	}
	return jwks.cacheMaxAge
}

// SigningKey returns the jwk.Key to be used for signing JWTs.
// This is the private key corresponding to one of the public keys in the JWKS.
func (jwks *JWKSService) SigningKey(_ context.Context) (jwk.Key, error) {
	if jwks == nil {
		return nil, errors.New("missing jwks service")
	}
	jwks.mu.RLock()
	defer jwks.mu.RUnlock()
	if jwks.cached.signingKey == nil {
		return nil, errors.New("missing signing key")
	}
	return jwks.cached.signingKey, nil
}

// Close stops the background JWKS refresh loop.
func (jwks *JWKSService) Close() error {
	if jwks == nil {
		return nil
	}
	jwks.closeOnce.Do(func() {
		close(jwks.stopCh)
		<-jwks.doneCh
	})
	return nil
}

func (jwks *JWKSService) refresh(ctx context.Context) {
	material, err := loadJWKS(jwks.jwksPath, jwks.privateKeyPath)
	if err != nil {
		zapctx.Warn(ctx, "failed to refresh jwks, using cached value", zap.Error(err))
		return
	}

	jwks.mu.Lock()
	defer jwks.mu.Unlock()
	jwks.cached = material
}

func (jwks *JWKSService) refreshLoop() {
	ticker := time.NewTicker(time.Duration(jwks.cacheMaxAge) * time.Second)
	defer ticker.Stop()
	defer close(jwks.doneCh)

	for {
		select {
		case <-ticker.C:
			jwks.refresh(context.Background())
		case <-jwks.stopCh:
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
	for i := 0; i < set.Len(); i++ {
		key, ok := set.Key(i)
		if !ok {
			continue
		}

		var publicKey rsa.PublicKey
		if err := key.Raw(&publicKey); err != nil {
			continue
		}
		if publicKey.E == signingPublicKey.E && publicKey.N.Cmp(signingPublicKey.N) == 0 {
			return key, nil
		}
	}

	return nil, errors.New("jwks does not contain the public key for the provided private key")
}
