// Copyright 2020 Canonical Ltd.

package dbmodel

import (
	"fmt"
	"time"

	"github.com/juju/names/v4"
	"gorm.io/gorm"

	apiparams "github.com/CanonicalLtd/jimm/api/params"
	jimmnames "github.com/CanonicalLtd/jimm/pkg/names"
)

// A GroupEntry holds information about a user group.
type GroupEntry struct {
	gorm.Model

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
	return jimmnames.NewGroupTag(fmt.Sprintf("%d", g.ID))
}
