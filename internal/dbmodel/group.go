// Copyright 2020 Canonical Ltd.

package dbmodel

import (
	"gorm.io/gorm"
)

// A GroupEntry holds information about a user group.
type GroupEntry struct {
	gorm.Model

	// Name holds the name of the group.
	Name string `gorm:"index;column:name"`
}

// TableName overrides the table name gorm will use to find
// GroupEntry records.
func (GroupEntry) TableName() string {
	return "groups"
}
