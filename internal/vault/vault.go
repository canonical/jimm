// Copyright 2021 Canonical Ltd.

package vault

import (
	"context"
	"net/http"
	"path"
	"sync"
	"time"

	"github.com/hashicorp/vault/api"
	"github.com/juju/names/v4"

	"github.com/canonical/jimm/internal/errors"
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
