// Copyright 2024 Canonical.

package names

import (
	"fmt"
	"regexp"
)

const (
	RoleTagKind = "role"
)

var (
	validRoleName      = regexp.MustCompile("^[a-zA-Z][a-zA-Z0-9._-]+[a-zA-Z0-9]$")
	validRoleIdSnippet = `^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}((#|\z)[a-z]+)?$`
	validRoleId        = regexp.MustCompile(validRoleIdSnippet)
)

// RoleTag represents a role.
// Implements juju names.Tag
type RoleTag struct {
	id string
}

// Id implements juju names.Tag
func (t RoleTag) Id() string { return t.id }

// Kind implements juju names.Tag
func (t RoleTag) Kind() string { return RoleTagKind }

// String implements juju names.Tag
func (t RoleTag) String() string { return RoleTagKind + "-" + t.Id() }

// NewRoleTag creates a valid RoleTag if it is possible to parse
// the provided tag.
func NewRoleTag(roleId string) RoleTag {
	id := validRoleId.FindString(roleId)

	if id == "" {
		panic(fmt.Sprintf("invalid role tag %q", roleId))
	}

	return RoleTag{id: id}
}

// ParseRoleTag parses a user role string.
func ParseRoleTag(tag string) (RoleTag, error) {
	t, err := ParseTag(tag)
	if err != nil {
		return RoleTag{}, err
	}
	gt, ok := t.(RoleTag)
	if !ok {
		return RoleTag{}, invalidTagError(tag, RoleTagKind)
	}
	return gt, nil
}

// IsValidRoleId verifies the id of the tag is valid according to a regex internally.
func IsValidRoleId(id string) bool {
	return validRoleId.MatchString(id)
}

// IsValidRoleName verifies the name of the role is valid
// according to the role name regexp.
// A valid role name:
// - starts with an upper- or lower-case character
// - ends with an upper- or lower-case character or a number
// - may contain ., _, or -
// - must at least 6 characters long.
func IsValidRoleName(name string) bool {
	return validRoleName.MatchString(name)
}
