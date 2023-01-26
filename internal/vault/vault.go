// Copyright 2021 Canonical Ltd.

package vault

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"path"
	"sync"
	"time"

	"github.com/google/uuid"
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

// PutJWKS puts a generated RS256[4096 bit] JWKS without x5c or x5t into the credential store.
//
// The pathing is similar to the controllers credentials
// in that we understand RS256 keys as credentials, rather than crytographic keys.
func (s *VaultStore) PutJWKS(ctx context.Context) error {
	const op = errors.Op("vault.PutJWKS")

	client, err := s.client(ctx)
	if err != nil {
		return errors.E(op, err)
	}

	jwks, err := s.generateJWKS(ctx)
	if err != nil {
		return errors.E(op, err)
	}

	ks := jwk.NewSet()
	err = ks.AddKey(jwks)
	if err != nil {
		return errors.E(op, err)
	}

	b, err := json.Marshal(ks)
	if err != nil {
		return errors.E(op, err)
	}

	_, err = client.Logical().WriteBytes(
		// We persist in a similar folder to the controller credentials, but sub-route
		// to .well-known for further extensions and mental clarity within our vault.
		s.getJWKSPath(),
		b,
	)
	if err != nil {
		return errors.E(op, err)
	}

	return nil
}

// StartJWKSRotator starts a simple routine which checks the vaults TTL for the JWKS on a defined CRON
// if the key set is within 1 day of expiry, it will rotate the keys.
func (s *VaultStore) StartJWKSRotator(ctx context.Context, cron string) error {
	return nil
}

// getJWKSPath returns a hardcoded suffixed vault path (dependent on
// the initial KVPath) to the .well-known JWKS location.
func (s *VaultStore) getJWKSPath() string {
	return path.Join(s.KVPath, "creds", ".well-known", "jwks")
}

// generateJWKS generates a new set of JWKS using RSA256[4096]
//
// TODO(ale8k)[possibly?]:
// For now, there's a single key, and this is probably OK. But possibly extend
// this to contain many at some point differentiated by KIDs.
//
// We also currently don't use x5c and x5t for validation and expect users
// to use e and n for validation.
// https://stackoverflow.com/questions/61395261/how-to-validate-signature-of-jwt-from-jwks-without-x5c
func (s *VaultStore) generateJWKS(ctx context.Context) (jwk.Key, error) {
	const op = errors.Op("vault.generateJWKS")

	// Due to the sensitivity of controllers, it is best we allow a larger encryption bit size
	// and accept any negligible wire cost.
	keySet, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		return nil, errors.E(op, err)
	}

	// We also use the same methodology of generating UUIDs for our KID
	kid, err := uuid.NewRandom()
	if err != nil {
		return nil, errors.E(op, err)
	}

	jwks, err := jwk.FromRaw(keySet)
	if err != nil {
		return nil, errors.E(op, err)
	}

	err = jwks.Set("kid", kid.String())
	if err != nil {
		return nil, errors.E(op, err)
	}

	err = jwks.Set("use", "sig")
	if err != nil {
		return nil, errors.E(op, err)
	}

	err = jwks.Set("alg", "RS256")
	if err != nil {
		return nil, errors.E(op, err)
	}

	return jwks, nil
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
