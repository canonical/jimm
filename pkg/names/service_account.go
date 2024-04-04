// Copyright 2024 canonical.

// Service accounts are an OIDC/OAuth concept which allows for machine<->machine communication.
// Service accounts are identified by their client ID.
package names

import (
	"fmt"

	"github.com/juju/names/v5"
)

const (
	// ServiceAccountTagKind represents the resource "kind" that service accounts
	// are represented as.
	ServiceAccountTagKind = "serviceaccount"
)

// ServiceAccount represents a service account where id is the client ID.
// Implements juju names.Tag.
type ServiceAccountTag struct {
	id string
}

// Id implements juju names.Tag.
func (t ServiceAccountTag) Id() string { return t.id }

// Kind implements juju names.Tag.
func (t ServiceAccountTag) Kind() string { return ServiceAccountTagKind }

// String implements juju names.Tag.
func (t ServiceAccountTag) String() string { return ServiceAccountTagKind + "-" + t.Id() }

// NewServiceAccountTag creates a valid ServiceAccountTag if it is possible to parse
// the provided tag.
func NewServiceAccountTag(clientId string) ServiceAccountTag {
	if !IsValidServiceAccountId(clientId) {
		panic(fmt.Sprintf("invalid client tag %q", clientId))
	}

	return ServiceAccountTag{id: clientId}
}

// ParseServiceAccountTag parses a service account tag string.
func ParseServiceAccountTag(tag string) (ServiceAccountTag, error) {
	t, err := ParseTag(tag)
	if err != nil {
		return ServiceAccountTag{}, err
	}
	gt, ok := t.(ServiceAccountTag)
	if !ok {
		return ServiceAccountTag{}, invalidTagError(tag, ServiceAccountTagKind)
	}
	return gt, nil
}

// IsValidServiceAccountId verifies the client id for a service account is valid according to a regex internally.
func IsValidServiceAccountId(id string) bool {
	if !names.IsValidUser(id) {
		return false
	}
	t := names.NewUserTag(id)
	return t.Domain() != ""
}
