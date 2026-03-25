// Copyright 2025 Canonical.

// Package credentials provides abstractions/definitions for credential storage
// backends and caching mechanisms.
package credentials

import (
	"context"

	"github.com/juju/names/v5"
)

// A CredentialStore is a store for the attributes of:
//   - Cloud credentials
//   - Controller credentials
type CredentialStore interface {
	// Get retrieves the stored attributes of a cloud credential.
	Get(context.Context, names.CloudCredentialTag) (map[string]string, error)

	// Put stores the attributes of a cloud credential.
	Put(context.Context, names.CloudCredentialTag, map[string]string) error

	// GetControllerCredentials retrieves the credentials for the given controller from a vault
	// service.
	GetControllerCredentials(ctx context.Context, controllerName string) (string, string, error)

	// PutControllerCredentials stores the controller credentials in a vault
	// service.
	PutControllerCredentials(ctx context.Context, controllerName string, username string, password string) error
}
