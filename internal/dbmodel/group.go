// Copyright 2020 Canonical Ltd.

package dbmodel

import (
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
	const time_format = "2006-01-02 15:04:05"
	group.Name = g.Name
	group.CreatedAt = g.CreatedAt.Format(time_format)
	group.UpdatedAt = g.UpdatedAt.Format(time_format)
	return group
}

// TableName overrides the table name gorm will use to find
// GroupEntry records.
func (GroupEntry) TableName() string {
	return "groups"
}
