// Copyright 2025 Canonical.

package vault

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"path"
	"sync"
	"time"

	"github.com/hashicorp/vault/api"
	auth "github.com/hashicorp/vault/api/auth/approle"
	"github.com/juju/names/v5"

	"github.com/canonical/jimm/v3/internal/servermon"
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
	const op = "vault.Get"

	durationObserver := servermon.DurationObserver(servermon.VaultCallDurationHistogram, op)
	defer durationObserver()
	defer servermon.ErrorCounter(servermon.VaultCallErrorCount, &err, op)

	client, err := s.client(ctx)
	if err != nil {
		return nil, err
	}

	secret, err := client.KVv2(s.KVPath).Get(ctx, s.path(tag))
	if err != nil && errors.Unwrap(err) != api.ErrSecretNotFound {
		return nil, err
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

	const op = "vault.Put"
	durationObserver := servermon.DurationObserver(servermon.VaultCallDurationHistogram, op)
	defer durationObserver()
	defer servermon.ErrorCounter(servermon.VaultCallErrorCount, &err, op)

	client, err := s.client(ctx)
	if err != nil {
		return err
	}

	data := make(map[string]any, len(attr))
	for k, v := range attr {
		data[k] = v
	}
	_, err = client.KVv2(s.KVPath).Put(ctx, s.path(tag), data)
	if err != nil {
		return err
	}
	return nil
}

// delete removes the attributes associated with the cloud-credential in
// the vault service.
func (s *VaultStore) delete(ctx context.Context, tag names.CloudCredentialTag) (err error) {
	const op = "vault.delete"
	durationObserver := servermon.DurationObserver(servermon.VaultCallDurationHistogram, op)
	defer durationObserver()
	defer servermon.ErrorCounter(servermon.VaultCallErrorCount, &err, op)

	client, err := s.client(ctx)
	if err != nil {
		return err
	}
	err = client.KVv2(s.KVPath).Delete(ctx, s.path(tag))
	if rerr, ok := err.(*api.ResponseError); ok && rerr.StatusCode == http.StatusNotFound {
		// Ignore the error if attempting to delete something that isn't there.
		err = nil
	}
	if err != nil {
		return err
	}
	return nil
}

// GetControllerCredentials retrieves the credentials for the given controller from a vault
// service.
func (s *VaultStore) GetControllerCredentials(ctx context.Context, controllerName string) (_ string, _ string, err error) {
	const op = "vault.GetControllerCredentials"

	durationObserver := servermon.DurationObserver(servermon.VaultCallDurationHistogram, op)
	defer durationObserver()
	defer servermon.ErrorCounter(servermon.VaultCallErrorCount, &err, op)

	client, err := s.client(ctx)
	if err != nil {
		return "", "", err
	}

	secret, err := client.KVv2(s.KVPath).Get(ctx, s.controllerCredentialsPath(controllerName))
	if err != nil && errors.Unwrap(err) != api.ErrSecretNotFound {
		return "", "", err
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

	const op = "vault.PutControllerCredentials"
	durationObserver := servermon.DurationObserver(servermon.VaultCallDurationHistogram, op)
	defer durationObserver()
	defer servermon.ErrorCounter(servermon.VaultCallErrorCount, &err, op)

	client, err := s.client(ctx)
	if err != nil {
		return err
	}

	data := map[string]any{
		usernameKey: username,
		passwordKey: password,
	}
	_, err = client.KVv2(s.KVPath).Put(ctx, s.controllerCredentialsPath(controllerName), data)
	if err != nil {
		return err
	}
	return nil
}

// deleteControllerCredentials removes the credentials associated with the controller in
// the vault service.
func (s *VaultStore) deleteControllerCredentials(ctx context.Context, controllerName string) (err error) {
	const op = "vault.deleteControllerCredentials"

	durationObserver := servermon.DurationObserver(servermon.VaultCallDurationHistogram, op)
	defer durationObserver()
	defer servermon.ErrorCounter(servermon.VaultCallErrorCount, &err, op)

	client, err := s.client(ctx)
	if err != nil {
		return err
	}
	err = client.KVv2(s.KVPath).Delete(ctx, s.controllerCredentialsPath(controllerName))
	if rerr, ok := err.(*api.ResponseError); ok && rerr.StatusCode == http.StatusNotFound {
		// Ignore the error if attempting to delete something that isn't there.
		err = nil
	}
	if err != nil {
		return err
	}
	return nil
}

const ttlLeeway time.Duration = 5 * time.Second

func (s *VaultStore) client(ctx context.Context) (*api.Client, error) {
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
		return nil, fmt.Errorf("unable to initialize approle auth method: %w", err)
	}

	authInfo, err := s.Client.Auth().Login(ctx, appRoleAuth)
	if err != nil {
		return nil, fmt.Errorf("unable to login to approle auth method: %w", err)
	}
	if authInfo == nil {
		return nil, errors.New("no auth info was returned after login")
	}

	ttl, err := authInfo.TokenTTL()
	if err != nil {
		return nil, err
	}
	tok, err := authInfo.TokenID()
	if err != nil {
		return nil, err
	}
	s.client_, err = s.Client.Clone()
	if err != nil {
		return nil, err
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
