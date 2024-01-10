// Service accounts are an OIDC/OAuth concept which allows for machine<->machine communication.
// Service accounts are identified by their client ID.
package names

import (
	"fmt"
	"regexp"
)

const (
	// ServiceAccountTagKind represents the resource "kind" that service accounts
	// are represented as.
	ServiceAccountTagKind = "serviceaccount"
)

var (
	validClientIdSnippet = `^[0-9a-zA-Z-]+$`
	validClientId        = regexp.MustCompile(validClientIdSnippet)
)

// ServiceAccount represents a service account where id is the client ID.
// Implements juju names.Tag
type ServiceAccountTag struct {
	id string
}

// Id implements juju names.Tag
func (t ServiceAccountTag) Id() string { return t.id }

// Kind implements juju names.Tag
func (t ServiceAccountTag) Kind() string { return ServiceAccountTagKind }

// String implements juju names.Tag
func (t ServiceAccountTag) String() string { return ServiceAccountTagKind + "-" + t.Id() }

// NewServiceAccountTag creates a valid ServiceAccountTag if it is possible to parse
// the provided tag.
func NewServiceAccountTag(clientId string) ServiceAccountTag {
	id := validClientId.FindString(clientId)

	if id == "" {
		panic(fmt.Sprintf("invalid client tag %q", clientId))
	}

	return ServiceAccountTag{id: id}
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
	return validClientId.MatchString(id)
}
