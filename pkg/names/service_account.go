// Copyright 2025 Canonical.

package names

import (
	"errors"
	"strings"

	"github.com/juju/names/v5"
)

// Service accounts interact with JIMM in the same way as users but in
// some scenarios, like during client-credential login, we allow the
// client to send only a service account ID without the `@serviceaccount` domain.
// This file provides constants and functions to validate and ensure that
// a service account ID is correctly formatted and has the appropriate domain.

const (
	// ServiceAccountDomain is the @domain suffix that service account IDs should
	// have.
	ServiceAccountDomain = "serviceaccount"
)

var (
	// ErrInvalidClientID indicates an invalid client ID error.
	ErrInvalidClientID = errors.New("invalid client ID")
)

// IsValidServiceAccountId verifies the client id for a service account is valid
// according to a regex internally. A valid service account ID must have a
// `@serviceaccount` domain.
func IsValidServiceAccountId(id string) bool {
	if !names.IsValidUser(id) {
		return false
	}
	t := names.NewUserTag(id)
	return t.Domain() == ServiceAccountDomain
}

// EnsureValidServiceAccountId returns the given service account ID with the
// `@serviceaccount` appended to it, if not already there. If the ID is not a
// valid service account ID this function returns an error.
func EnsureValidServiceAccountId(id string) (string, error) {
	if !strings.HasSuffix(id, "@"+ServiceAccountDomain) {
		id += "@" + ServiceAccountDomain
	}

	if !IsValidServiceAccountId(id) {
		return "", ErrInvalidClientID
	}
	return id, nil
}
