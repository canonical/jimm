// Copyright 2024 Canonical.

package vault

import (
	"context"
	"encoding/base64"
	"encoding/json"
	goerr "errors"
	"net/http"
	"path"
	"sync"
	"time"

	"github.com/hashicorp/vault/api"
	auth "github.com/hashicorp/vault/api/auth/approle"
	"github.com/juju/names/v5"
	"github.com/juju/zaputil/zapctx"
	"github.com/lestrrat-go/jwx/v2/jwk"
	"go.uber.org/zap"

	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/servermon"
)

const (
	usernameKey = "username"
	passwordKey = "password"
)

const (
	jwksKey        = "jwks"
	jwksExpiryKey  = "jwks-expiry"
	jwksPrivateKey = "jwks-private"
	oAuthSecretKey = "oauth-secret"
)

// A VaultStore stores cloud credential attributes and
// controller credentials in vault.
type VaultStore struct {
	// Client contains the client used to communicate with the vault
	// service. This client is not modified by the store.
	Client *api.Client

	// RoleID is the AppRole role ID.
	RoleID string

	// RoleSecretID is the AppRole secret ID.
	RoleSecretID string

	// KVPath is the root path in the vault for JIMM's key-value
	// storage.
	KVPath string

	// mu protects the fields below it.
	mu      sync.Mutex
	expires time.Time
	client_ *api.Client
}

// Get retrieves the attributes for the given cloud credential from a vault
// service.
func (s *VaultStore) Get(ctx context.Context, tag names.CloudCredentialTag) (_ map[string]string, err error) {
	const op = errors.Op("vault.Get")

	durationObserver := servermon.DurationObserver(servermon.VaultCallDurationHistogram, string(op))
	defer durationObserver()
	defer servermon.ErrorCounter(servermon.VaultCallErrorCount, &err, string(op))

	client, err := s.client(ctx)
	if err != nil {
		return nil, errors.E(op, err)
	}

	secret, err := client.KVv2(s.KVPath).Get(ctx, s.path(tag))
	if err != nil && goerr.Unwrap(err) != api.ErrSecretNotFound {
		return nil, errors.E(op, err)
	}
	if secret == nil || secret.Data == nil {
		return nil, nil
	}
	attr := make(map[string]string, len(secret.Data))
	for k, v := range secret.Data {
		// Nothing will be stored that isn't a string, so ignore anything
		// that is a different type.
		s, ok := v.(string)
		if !ok {
			continue
		}
		attr[k] = s
	}
	return attr, nil
}

// Put stores the attributes associated with a cloud-credential in a vault
// service.
func (s *VaultStore) Put(ctx context.Context, tag names.CloudCredentialTag, attr map[string]string) (err error) {
	if len(attr) == 0 {
		return s.delete(ctx, tag)
	}

	const op = errors.Op("vault.Put")

	durationObserver := servermon.DurationObserver(servermon.VaultCallDurationHistogram, string(op))
	defer durationObserver()
	defer servermon.ErrorCounter(servermon.VaultCallErrorCount, &err, string(op))

	client, err := s.client(ctx)
	if err != nil {
		return errors.E(op, err)
	}

	data := make(map[string]interface{}, len(attr))
	for k, v := range attr {
		data[k] = v
	}
	_, err = client.KVv2(s.KVPath).Put(ctx, s.path(tag), data)
	if err != nil {
		return errors.E(op, err)
	}
	return nil
}

// delete removes the attributes associated with the cloud-credential in
// the vault service.
func (s *VaultStore) delete(ctx context.Context, tag names.CloudCredentialTag) (err error) {
	const op = errors.Op("vault.delete")

	durationObserver := servermon.DurationObserver(servermon.VaultCallDurationHistogram, string(op))
	defer durationObserver()
	defer servermon.ErrorCounter(servermon.VaultCallErrorCount, &err, string(op))

	client, err := s.client(ctx)
	if err != nil {
		return errors.E(op, err)
	}
	err = client.KVv2(s.KVPath).Delete(ctx, s.path(tag))
	if rerr, ok := err.(*api.ResponseError); ok && rerr.StatusCode == http.StatusNotFound {
		// Ignore the error if attempting to delete something that isn't there.
		err = nil
	}
	if err != nil {
		return errors.E(op, err)
	}
	return nil
}

// GetControllerCredentials retrieves the credentials for the given controller from a vault
// service.
func (s *VaultStore) GetControllerCredentials(ctx context.Context, controllerName string) (_ string, _ string, err error) {
	const op = errors.Op("vault.GetControllerCredentials")

	durationObserver := servermon.DurationObserver(servermon.VaultCallDurationHistogram, string(op))
	defer durationObserver()
	defer servermon.ErrorCounter(servermon.VaultCallErrorCount, &err, string(op))

	client, err := s.client(ctx)
	if err != nil {
		return "", "", errors.E(op, err)
	}

	secret, err := client.KVv2(s.KVPath).Get(ctx, s.controllerCredentialsPath(controllerName))
	if err != nil && goerr.Unwrap(err) != api.ErrSecretNotFound {
		return "", "", errors.E(op, err)
	}
	if secret == nil || secret.Data == nil {
		return "", "", nil
	}
	var username, password string
	usernameI, ok := secret.Data[usernameKey]
	if ok {
		username = usernameI.(string)
	}
	passwordI, ok := secret.Data[passwordKey]
	if ok {
		password = passwordI.(string)
	}
	return username, password, nil
}

// PutControllerCredentials stores the controller credentials in a vault
// service.
func (s *VaultStore) PutControllerCredentials(ctx context.Context, controllerName string, username string, password string) (err error) {
	if username == "" || password == "" {
		return s.deleteControllerCredentials(ctx, controllerName)
	}

	const op = errors.Op("vault.PutControllerCredentials")

	durationObserver := servermon.DurationObserver(servermon.VaultCallDurationHistogram, string(op))
	defer durationObserver()
	defer servermon.ErrorCounter(servermon.VaultCallErrorCount, &err, string(op))

	client, err := s.client(ctx)
	if err != nil {
		return errors.E(op, err)
	}

	data := map[string]interface{}{
		usernameKey: username,
		passwordKey: password,
	}
	_, err = client.KVv2(s.KVPath).Put(ctx, s.controllerCredentialsPath(controllerName), data)
	if err != nil {
		return errors.E(op, err)
	}
	return nil
}

// CleanupJWKS removes all secrets associated with the JWKS process.
func (s *VaultStore) CleanupJWKS(ctx context.Context) (err error) {
	const op = errors.Op("vault.CleanupJWKS")

	durationObserver := servermon.DurationObserver(servermon.VaultCallDurationHistogram, string(op))
	defer durationObserver()
	defer servermon.ErrorCounter(servermon.VaultCallErrorCount, &err, string(op))

	client, err := s.client(ctx)
	if err != nil {
		return errors.E(op, err)
	}

	if err = client.KVv2(s.KVPath).Delete(ctx, s.getJWKSExpiryPath()); err != nil {
		return errors.E(op, err)
	}

	if err = client.KVv2(s.KVPath).Delete(ctx, s.getJWKSPath()); err != nil {
		return errors.E(op, err)
	}

	if err = client.KVv2(s.KVPath).Delete(ctx, s.getJWKSPrivateKeyPath()); err != nil {
		return errors.E(op, err)
	}
	return nil
}

// GetJWKS returns the current key set stored within the credential store.
func (s *VaultStore) GetJWKS(ctx context.Context) (_ jwk.Set, err error) {
	const op = errors.Op("vault.GetJWKS")

	durationObserver := servermon.DurationObserver(servermon.VaultCallDurationHistogram, string(op))
	defer durationObserver()
	defer servermon.ErrorCounter(servermon.VaultCallErrorCount, &err, string(op))

	client, err := s.client(ctx)
	if err != nil {
		return nil, errors.E(op, err)
	}

	secret, err := client.KVv2(s.KVPath).Get(ctx, s.getJWKSPath())
	if err != nil && goerr.Unwrap(err) != api.ErrSecretNotFound {
		return nil, errors.E(op, err)
	}

	// This is how the current version of vaults API Read works,
	// if the secret is not present on the path, it will instead return
	// nil for the secret and a nil error. So we must check for this.
	if secret == nil || secret.Data == nil {
		msg := "no JWKS exists yet."
		zapctx.Debug(ctx, msg)
		return nil, errors.E(op, errors.CodeNotFound, msg)
	}

	jsonString, ok := secret.Data[jwksKey].(string)
	if !ok {
		return nil, errors.E(op, "invalid type for jwks")
	}

	ks, err := jwk.ParseString(jsonString)
	if err != nil {
		return nil, errors.E(op, err)
	}

	return ks, nil
}

// GetJWKSPrivateKey returns the current private key for the active JWKS
func (s *VaultStore) GetJWKSPrivateKey(ctx context.Context) (_ []byte, err error) {
	const op = errors.Op("vault.GetJWKSPrivateKey")

	durationObserver := servermon.DurationObserver(servermon.VaultCallDurationHistogram, string(op))
	defer durationObserver()
	defer servermon.ErrorCounter(servermon.VaultCallErrorCount, &err, string(op))

	client, err := s.client(ctx)
	if err != nil {
		return nil, errors.E(op, err)
	}

	secret, err := client.KVv2(s.KVPath).Get(ctx, s.getJWKSPrivateKeyPath())
	if err != nil && goerr.Unwrap(err) != api.ErrSecretNotFound {
		return nil, errors.E(op, err)
	}

	if secret == nil || secret.Data == nil {
		msg := "no JWKS private key exists yet."
		zapctx.Debug(ctx, msg)
		return nil, errors.E(op, errors.CodeNotFound, msg)
	}

	keyPemB64 := secret.Data[jwksPrivateKey].(string)

	keyPem, err := base64.StdEncoding.DecodeString(keyPemB64)
	if err != nil {
		return nil, errors.E(op, err)
	}

	return keyPem, nil
}

// GetJWKSExpiry returns the expiry of the active JWKS.
func (s *VaultStore) GetJWKSExpiry(ctx context.Context) (_ time.Time, err error) {
	const op = errors.Op("vault.getJWKSExpiry")

	durationObserver := servermon.DurationObserver(servermon.VaultCallDurationHistogram, string(op))
	defer durationObserver()
	defer servermon.ErrorCounter(servermon.VaultCallErrorCount, &err, string(op))

	now := time.Now()
	client, err := s.client(ctx)
	if err != nil {
		return now, errors.E(op, err)
	}

	secret, err := client.KVv2(s.KVPath).Get(ctx, s.getJWKSExpiryPath())
	if err != nil && goerr.Unwrap(err) != api.ErrSecretNotFound {
		return now, errors.E(op, err)
	}

	if secret == nil || secret.Data == nil {
		msg := "no JWKS expiry exists yet."
		zapctx.Debug(ctx, msg)
		return now, errors.E(op, errors.CodeNotFound, msg)
	}

	expiry, ok := secret.Data[jwksExpiryKey].(string)
	if !ok {
		return now, errors.E(op, "failed to retrieve expiry")
	}

	t, err := time.Parse(time.RFC3339, expiry)
	if err != nil {
		return now, errors.E(op, err)
	}

	return t, nil
}

// PutJWKS puts a JWKS into the credential store.
//
// The pathing is similar to the controllers credentials
// in that we understand RS256 keys as credentials, rather than crytographic keys.
func (s *VaultStore) PutJWKS(ctx context.Context, jwks jwk.Set) (err error) {
	const op = errors.Op("vault.PutJWKS")

	durationObserver := servermon.DurationObserver(servermon.VaultCallDurationHistogram, string(op))
	defer durationObserver()
	defer servermon.ErrorCounter(servermon.VaultCallErrorCount, &err, string(op))

	client, err := s.client(ctx)
	if err != nil {
		return errors.E(op, err)
	}

	jwksJson, err := json.Marshal(jwks)
	if err != nil {
		return errors.E(op, err)
	}

	jwksData := map[string]any{jwksKey: string(jwksJson)}
	if _, err = client.KVv2(s.KVPath).Put(ctx, s.getJWKSPath(), jwksData); err != nil {
		return errors.E(op, err)
	}

	return nil
}

// PutJWKSPrivateKey persists the private key associated with the current JWKS within the store.
func (s *VaultStore) PutJWKSPrivateKey(ctx context.Context, pem []byte) (err error) {
	const op = errors.Op("vault.PutJWKSPrivateKey")

	durationObserver := servermon.DurationObserver(servermon.VaultCallDurationHistogram, string(op))
	defer durationObserver()
	defer servermon.ErrorCounter(servermon.VaultCallErrorCount, &err, string(op))

	client, err := s.client(ctx)
	if err != nil {
		return errors.E(op, err)
	}

	privateKeyData := map[string]interface{}{jwksPrivateKey: pem}
	if _, err := client.KVv2(s.KVPath).Put(ctx, s.getJWKSPrivateKeyPath(), privateKeyData); err != nil {
		return errors.E(op, err)
	}
	return nil
}

// PutJWKSExpiry sets the expiry time for the current JWKS within the store.
func (s *VaultStore) PutJWKSExpiry(ctx context.Context, expiry time.Time) (err error) {
	const op = errors.Op("vault.PutJWKSExpiry")

	durationObserver := servermon.DurationObserver(servermon.VaultCallDurationHistogram, string(op))
	defer durationObserver()
	defer servermon.ErrorCounter(servermon.VaultCallErrorCount, &err, string(op))

	client, err := s.client(ctx)
	if err != nil {
		return errors.E(op, err)
	}
	expiryData := map[string]interface{}{jwksExpiryKey: expiry}
	if _, err := client.KVv2(s.KVPath).Put(ctx, s.getJWKSExpiryPath(), expiryData); err != nil {
		return errors.E(op, err)
	}
	return nil
}

// getWellKnownPath returns a hard coded path to the .well-known credentials.
func (s *VaultStore) getWellKnownPath() string {
	return path.Join("creds", ".well-known")
}

// getJWKSPath returns a hardcoded suffixed vault path (dependent on
// the initial KVPath) to the .well-known JWKS location.
func (s *VaultStore) getJWKSPath() string {
	return path.Join(s.getWellKnownPath(), "jwks.json")
}

// getJWKSPath returns a hardcoded suffixed vault path (dependent on
// the initial KVPath) to the .well-known JWKS location.
func (s *VaultStore) getJWKSPrivateKeyPath() string {
	return path.Join(s.getWellKnownPath(), "jwks-key.pem")
}

// getJWKSPath returns the path to the jwks expiry secret.
func (s *VaultStore) getJWKSExpiryPath() string {
	return path.Join(s.getWellKnownPath(), "jwks-expiry")
}

// deleteControllerCredentials removes the credentials associated with the controller in
// the vault service.
func (s *VaultStore) deleteControllerCredentials(ctx context.Context, controllerName string) (err error) {
	const op = errors.Op("vault.deleteControllerCredentials")

	durationObserver := servermon.DurationObserver(servermon.VaultCallDurationHistogram, string(op))
	defer durationObserver()
	defer servermon.ErrorCounter(servermon.VaultCallErrorCount, &err, string(op))

	client, err := s.client(ctx)
	if err != nil {
		return errors.E(op, err)
	}
	err = client.KVv2(s.KVPath).Delete(ctx, s.controllerCredentialsPath(controllerName))
	if rerr, ok := err.(*api.ResponseError); ok && rerr.StatusCode == http.StatusNotFound {
		// Ignore the error if attempting to delete something that isn't there.
		err = nil
	}
	if err != nil {
		return errors.E(op, err)
	}
	return nil
}

const ttlLeeway time.Duration = 5 * time.Second

func (s *VaultStore) client(ctx context.Context) (*api.Client, error) {
	const op = errors.Op("vault.client")

	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	if now.Before(s.expires) {
		return s.client_, nil
	}

	roleSecretID := &auth.SecretID{
		FromString: s.RoleSecretID,
	}
	appRoleAuth, err := auth.NewAppRoleAuth(
		s.RoleID,
		roleSecretID,
	)
	if err != nil {
		zapctx.Error(ctx, "unable to initialize approle auth method", zap.Error(err))
		return nil, errors.E(op, err, "unable to initialize approle auth method")
	}

	authInfo, err := s.Client.Auth().Login(ctx, appRoleAuth)
	if err != nil {
		zapctx.Error(ctx, "unable to login to approle auth method", zap.Error(err))
		return nil, errors.E(op, err, "unable to login to approle auth method")
	}
	if authInfo == nil {
		return nil, errors.E(op, "no auth info was returned after login")
	}

	ttl, err := authInfo.TokenTTL()
	if err != nil {
		return nil, errors.E(op, err)
	}
	tok, err := authInfo.TokenID()
	if err != nil {
		return nil, errors.E(op, err)
	}
	s.client_, err = s.Client.Clone()
	if err != nil {
		return nil, errors.E(op, err)
	}
	s.client_.SetToken(tok)
	s.expires = now.Add(ttl - ttlLeeway)
	return s.client_, nil
}

func (s *VaultStore) path(tag names.CloudCredentialTag) string {
	return path.Join("creds", tag.Cloud().Id(), tag.Owner().Id(), tag.Name())
}

func (s *VaultStore) controllerCredentialsPath(controllerName string) string {
	return path.Join("creds", controllerName)
}
