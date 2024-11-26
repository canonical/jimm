// Copyright 2024 Canonical.

package dbmodel

import (
	"time"

	"github.com/juju/names/v5"

	jimmnames "github.com/canonical/jimm/v3/pkg/names"
)

// RoleEntry holds a role entry.
type RoleEntry struct {
	// Note this doesn't use the standard gorm.Model to avoid soft-deletes.
	ID        uint `gorm:"primarykey"`
	CreatedAt time.Time
	UpdatedAt time.Time

	// Name holds the name of the role.
	Name string `gorm:"index;column:name"`

	// UUID holds the uuid of the role.
	UUID string `gorm:"index;column:uuid"`
}

// TableName overrides the table name gorm will use to find
// RoleEntry records.
func (*RoleEntry) TableName() string {
	return "roles"
}

// Tag implements the names.Tag interface.
func (r *RoleEntry) Tag() names.Tag {
	return r.ResourceTag()
}

// ResourceTag returns a tag for this role. This method
// is intended to be used in places where we expect to see
// a concrete type names.RoleTag instead of the
// names.Tag interface.
func (r *RoleEntry) ResourceTag() jimmnames.RoleTag {
	return jimmnames.NewRoleTag(r.UUID)
}
