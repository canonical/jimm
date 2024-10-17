// Copyright 2024 Canonical.

package dbmodel

import (
	"time"

	"github.com/juju/names/v5"

	apiparams "github.com/canonical/jimm/v3/pkg/api/params"
	jimmnames "github.com/canonical/jimm/v3/pkg/names"
)

// A GroupEntry holds information about a user group.
type GroupEntry struct {
	// Note this doesn't use the standard gorm.Model to avoid soft-deletes.
	ID        uint `gorm:"primarykey"`
	CreatedAt time.Time
	UpdatedAt time.Time

	// Name holds the name of the group.
	Name string `gorm:"index;column:name"`

	// UUID holds the uuid of the group.
	UUID string `gotm:"index;column:uuid"`
}

// ToAPIGroup converts a group entry to a JIMM API
// Group.
func (g GroupEntry) ToAPIGroupEntry() apiparams.Group {
	var group apiparams.Group
	group.UUID = g.UUID
	group.Name = g.Name
	group.CreatedAt = g.CreatedAt.Format(time.RFC3339)
	group.UpdatedAt = g.UpdatedAt.Format(time.RFC3339)
	return group
}

// TableName overrides the table name gorm will use to find
// GroupEntry records.
func (GroupEntry) TableName() string {
	return "groups"
}

// Tag implements the names.Tag interface.
func (g *GroupEntry) Tag() names.Tag {
	return g.ResourceTag()
}

// ResourceTag returns a tag for this group. This method
// is intended to be used in places where we expect to see
// a concrete type names.GroupTag instead of the
// names.Tag interface.
func (g *GroupEntry) ResourceTag() jimmnames.GroupTag {
	return jimmnames.NewGroupTag(g.UUID)
}
