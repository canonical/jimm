// Copyright 2026 Canonical.

package names

import (
	"fmt"
	"strings"
)

const (
	IdPGroupTagKind = "idpgroup"
)

// IdPGroupTag represents a group owned by an external identity provider.
// Implements juju names.Tag.
type IdPGroupTag struct {
	id string
}

// Id implements juju names.Tag.
func (t IdPGroupTag) Id() string { return t.id }

// Kind implements juju names.Tag.
func (t IdPGroupTag) Kind() string { return IdPGroupTagKind }

// String implements juju names.Tag.
func (t IdPGroupTag) String() string { return IdPGroupTagKind + "-" + t.Id() }

// NewIdPGroupTag creates an IdPGroupTag from the provided external group identifier.
func NewIdPGroupTag(groupId string) IdPGroupTag {
	if !IsValidIdPGroupId(groupId) {
		panic(fmt.Sprintf("invalid idp group tag %q", groupId))
	}

	return IdPGroupTag{id: groupId}
}

// ParseIdPGroupTag parses an IDP group string.
func ParseIdPGroupTag(tag string) (IdPGroupTag, error) {
	t, err := ParseTag(tag)
	if err != nil {
		return IdPGroupTag{}, err
	}
	gt, ok := t.(IdPGroupTag)
	if !ok {
		return IdPGroupTag{}, invalidTagError(tag, IdPGroupTagKind)
	}
	return gt, nil
}

// IsValidIdPGroupId verifies the id is non-empty and does not contain multiple relation separators.
func IsValidIdPGroupId(id string) bool {
	if id == "" {
		return false
	}
	parts := strings.Split(id, "#")
	if len(parts) > 2 {
		return false
	}
	if parts[0] == "" {
		return false
	}
	return len(parts) == 1 || parts[1] != ""
}
