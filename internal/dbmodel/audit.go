// Copyright 2020 Canonical Ltd.

package dbmodel

import (
	"time"

	"gorm.io/gorm"

	apiparams "github.com/canonical/jimm/api/params"
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

	// Success indicates whether the action succeeded, or not.
	Success bool

	// Params contains the event-specific params for the audit entry.
	Params StringMap
}

// TableName overrides the table name gorm will use to find
// AuditLogEntry records.
func (AuditLogEntry) TableName() string {
	return "audit_log"
}

// ToAPIAuditEvent convers an AuditLogEntry to a JIMM API AuditEvent.
func (e AuditLogEntry) ToAPIAuditEvent() apiparams.AuditEvent {
	var ale apiparams.AuditEvent
	ale.Time = e.Time
	ale.Tag = e.Tag
	ale.UserTag = e.UserTag
	ale.Action = e.Action
	ale.Success = e.Success
	ale.Params = make(map[string]string)
	for k, v := range e.Params {
		ale.Params[k] = v
	}
	return ale
}
