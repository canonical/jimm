// Copyright 2020 Canonical Ltd.

package dbmodel

import (
	"strconv"
	"time"

	"github.com/juju/names/v4"

	apiparams "github.com/canonical/jimm/api/params"
	jimmnames "github.com/canonical/jimm/pkg/names"
)

// A GroupEntry holds information about a user group.
type GroupEntry struct {
	ID        uint `gorm:"primaryKey"`
	CreatedAt time.Time
	UpdatedAt time.Time

	// Name holds the name of the group.
	Name string `gorm:"index;column:name"`
}

// ToAPIGroup converts a group entry to a JIMM API
// Group.
func (g GroupEntry) ToAPIGroupEntry() apiparams.Group {
	var group apiparams.Group
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
	return jimmnames.NewGroupTag(strconv.Itoa(int(g.ID)))
}
