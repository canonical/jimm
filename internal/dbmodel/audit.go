// Copyright 2020 Canonical Ltd.

package dbmodel

import (
	"time"

	"gorm.io/gorm"
)

// An AuditLogEntry is an entry in the audit log.
type AuditLogEntry struct {
	gorm.Model

	// Time contains the time that the event happened.
	Time time.Time `gorm:"index"`

	// Tag is the tag of the entity that this audit entry is for.
	Tag string `gorm:"index"`

	// UserTag is the tag of the user the performed the action.
	UserTag string `gorm:"index"`

	// Action is the type of event that this audit entry is for.
	Action string `gorm:"index"`

	// Params contains the event-specific params for the audit entry.
	Params StringMap
}
