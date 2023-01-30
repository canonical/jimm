// Copyright 2021 Canonical Ltd.

package vault

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"path"
	"sync"
	"time"

	"github.com/hashicorp/vault/api"
	"github.com/juju/names/v4"
	"github.com/juju/zaputil/zapctx"
	"github.com/lestrrat-go/jwx/v2/jwk"

	"github.com/CanonicalLtd/jimm/internal/errors"
)

const (
	usernameKey = "username"
	passwordKey = "password"
)

// A VaultStore stores cloud credential attributes and
// controller credentials in vault.
type VaultStore struct {
	// Client contains the client used to communicate with the vault
	// service. This client is not modified by the store.
	Client *api.Client

	// AuthSecret contains the secret used to authenticate with the
	// vault service.
	AuthSecret map[string]interface{}

	// AuthPath is the path of the endpoint used to authenticate with
	// the vault service.
	AuthPath string

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
func (s *VaultStore) Get(ctx context.Context, tag names.CloudCredentialTag) (map[string]string, error) {
	const op = errors.Op("vault.Get")

	client, err := s.client(ctx)
	if err != nil {
		return nil, errors.E(op, err)
	}

	secret, err := client.Logical().Read(s.path(tag))
	if err != nil {
		return nil, errors.E(op, err)
	}
	if secret == nil {
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
func (s *VaultStore) Put(ctx context.Context, tag names.CloudCredentialTag, attr map[string]string) error {
	if len(attr) == 0 {
		return s.delete(ctx, tag)
	}

	const op = errors.Op("vault.Put")
	client, err := s.client(ctx)
	if err != nil {
		return errors.E(op, err)
	}

	data := make(map[string]interface{}, len(attr))
	for k, v := range attr {
		data[k] = v
	}
	_, err = client.Logical().Write(s.path(tag), data)
	if err != nil {
		return errors.E(op, err)
	}
	return nil
}

// delete removes the attributes associated with the cloud-credential in
// the vault service.
func (s *VaultStore) delete(ctx context.Context, tag names.CloudCredentialTag) error {
	const op = errors.Op("vault.delete")

	client, err := s.client(ctx)
	if err != nil {
		return errors.E(op, err)
	}
	_, err = client.Logical().Delete(s.path(tag))
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
func (s *VaultStore) GetControllerCredentials(ctx context.Context, controllerName string) (string, string, error) {
	const op = errors.Op("vault.GetControllerCredentials")

	client, err := s.client(ctx)
	if err != nil {
		return "", "", errors.E(op, err)
	}

	secret, err := client.Logical().Read(s.controllerCredentialsPath(controllerName))
	if err != nil {
		return "", "", errors.E(op, err)
	}
	if secret == nil {
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
func (s *VaultStore) PutControllerCredentials(ctx context.Context, controllerName string, username string, password string) error {
	if username == "" || password == "" {
		return s.deleteControllerCredentials(ctx, controllerName)
	}

	const op = errors.Op("vault.PutControllerCredentials")
	client, err := s.client(ctx)
	if err != nil {
		return errors.E(op, err)
	}

	data := map[string]interface{}{
		usernameKey: username,
		passwordKey: password,
	}
	_, err = client.Logical().Write(s.controllerCredentialsPath(controllerName), data)
	if err != nil {
		return errors.E(op, err)
	}
	return nil
}

// GetJWKS returns the current key set stored within the credential store.
func (s *VaultStore) GetJWKS(ctx context.Context) (jwk.Set, error) {
	const op = errors.Op("vault.GetJWKS")

	client, err := s.client(ctx)
	if err != nil {
		return nil, errors.E(op, err)
	}

	secret, err := client.Logical().Read(s.getJWKSPath())
	if err != nil {
		return nil, errors.E(op, err)
	}

	// This is how the current version of vaults API Read works,
	// if the secret is not present on the path, it will instead return
	// nil for the secret and a nil error. So we must check for this.
	if secret == nil {
		msg := "no JWKS exists yet."
		zapctx.Debug(ctx, msg)
		return nil, errors.E(op, "no jwks exists", msg)
	}

	b, err := json.Marshal(secret.Data)
	if err != nil {
		return nil, errors.E(op, err)
	}

	ks, err := jwk.ParseString(string(b))
	if err != nil {
		return nil, errors.E(op, err)
	}

	return ks, nil
}

// GetJWKSPrivateKey returns the current private key for the active JWKS
func (s *VaultStore) GetJWKSPrivateKey(ctx context.Context) ([]byte, error) {
	const op = errors.Op("vault.GetJWKSPrivateKey")

	client, err := s.client(ctx)
	if err != nil {
		return nil, errors.E(op, err)
	}

	secret, err := client.Logical().Read(s.getJWKSPrivateKeyPath())
	if err != nil {
		return nil, errors.E(op, err)
	}

	if secret == nil {
		msg := "no JWKS exists yet."
		zapctx.Debug(ctx, msg)
		return nil, errors.E(op, "no jwks exists", msg)
	}

	keyPemB64 := secret.Data["key"].(string)

	keyPem, err := base64.StdEncoding.DecodeString(keyPemB64)
	if err != nil {
		return nil, errors.E(op, err)
	}

	return keyPem, nil
}

// GetJWKSExpiry returns the current expiry for JIMM's JWKS.
func (s *VaultStore) GetJWKSExpiry(ctx context.Context) (time.Time, error) {
	const op = errors.Op("vault.getJWKSExpiry")
	now := time.Now()
	client, err := s.client(ctx)
	if err != nil {
		return now, errors.E(op, err)
	}

	secret, err := client.Logical().Read(s.getJWKSExpiryPath())
	if err != nil {
		return now, errors.E(op, err)
	}

	if secret == nil {
		msg := "no JWKS exists yet."
		zapctx.Debug(ctx, msg)
		return now, errors.E(op, "no jwks exists", msg)
	}

	expiry, ok := secret.Data["jwks-expiry"].(string)
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
func (s *VaultStore) PutJWKS(ctx context.Context, jwks jwk.Set) error {
	const op = errors.Op("vault.PutJWKS")

	client, err := s.client(ctx)
	if err != nil {
		return errors.E(op, err)
	}

	jwksJson, err := json.Marshal(jwks)
	if err != nil {
		return errors.E(op, err)
	}

	_, err = client.Logical().WriteBytes(
		// We persist in a similar folder to the controller credentials, but sub-route
		// to .well-known for further extensions and mental clarity within our vault.
		s.getJWKSPath(),
		jwksJson,
	)
	if err != nil {
		return errors.E(op, err)
	}

	return nil
}

// PutJWKSPrivateKey persists the private key associated with the current JWKS within the store.
func (s *VaultStore) PutJWKSPrivateKey(ctx context.Context, pem []byte) error {
	const op = errors.Op("vault.PutJWKSPrivateKey")

	client, err := s.client(ctx)
	if err != nil {
		return errors.E(op, err)
	}

	if _, err := client.Logical().Write(
		// We persist in a similar folder to the controller credentials, but sub-route
		// to .well-known for further extensions and mental clarity within our vault.
		s.getJWKSPrivateKeyPath(),
		map[string]interface{}{"key": pem},
	); err != nil {
		return errors.E(op, err)
	}
	return nil
}

// PutJWKSExpiry sets the expiry time for the current JWKS within the store.
func (s *VaultStore) PutJWKSExpiry(ctx context.Context, expiry time.Time) error {
	const op = errors.Op("vault.PutJWKSExpiry")

	client, err := s.client(ctx)
	if err != nil {
		return errors.E(op, err)
	}

	if _, err := client.Logical().Write(
		s.getJWKSExpiryPath(),
		map[string]interface{}{
			"jwks-expiry": expiry,
		},
	); err != nil {
		return errors.E(op, err)
	}
	return nil
}

// getWellKnownPath returns a hard coded path to the .well-known credentials.
func (s *VaultStore) getWellKnownPath() string {
	return path.Join(s.KVPath, "creds", ".well-known")
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
func (s *VaultStore) deleteControllerCredentials(ctx context.Context, controllerName string) error {
	const op = errors.Op("vault.deleteControllerCredentials")

	client, err := s.client(ctx)
	if err != nil {
		return errors.E(op, err)
	}
	_, err = client.Logical().Delete(s.controllerCredentialsPath(controllerName))
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

	secret, err := s.Client.Logical().Write(s.AuthPath, s.AuthSecret)
	if err != nil {
		return nil, errors.E(op, err)
	}
	ttl, err := secret.TokenTTL()
	if err != nil {
		return nil, errors.E(op, err)
	}
	tok, err := secret.TokenID()
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
	return path.Join(s.KVPath, "creds", tag.Cloud().Id(), tag.Owner().Id(), tag.Name())
}

func (s *VaultStore) controllerCredentialsPath(controllerName string) string {
	return path.Join(s.KVPath, "creds", controllerName)
}
