// Copyright 2020 Canonical Ltd.

package dbmodel

import (
	"time"

	apiparams "github.com/CanonicalLtd/jimm/api/params"
	"gorm.io/gorm"
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
