// Copyright 2020 Canonical Ltd.

package dbmodel

import (
	"encoding/json"
	"time"

	apiparams "github.com/canonical/jimm/api/params"
)

// An AuditLogEntry is an entry in the audit log.
type AuditLogEntry struct {
	// ID contains the ID of the entry.
	ID uint `gorm:"primarykey"`

	// Time holds the time of event creation.
	Time time.Time `gorm:"index"`

	// Model contains the name of the model accessed.
	// Will be empty when accessing controller facades, as they are handled
	// by JIMM.
	Model string `gorm:"index"`

	// ConversationId contains a unique ID per websocket request.
	ConversationId string

	// MessageId represents the message ID used to correlate request/responses.
	MessageId uint64

	// FacadeName contains the request facade name.
	FacadeName string

	// FacadeMethod contains the specific method to be executed on the facade.
	FacadeMethod string `gorm:"index"`

	// FacadeVersion contains the requested version for the facade method.
	FacadeVersion int

	// ObjectId contains the object id to act on, only used by certain facades.
	ObjectId string

	// IdentityTag is the tag of the identity that performed the action.
	IdentityTag string `gorm:"index"`

	// IsResponse indicates whether the action was a Response/Request.
	IsResponse bool

	// Params contains any client request parameters.
	Params JSON

	// Errors contains any errors from the controller.
	Errors JSON
}

// TableName overrides the table name gorm will use to find
// AuditLogEntry records.
func (AuditLogEntry) TableName() string {
	return "audit_log"
}

// ToAPIAuditEvent converts an AuditLogEntry to a JIMM API AuditEvent.
func (e AuditLogEntry) ToAPIAuditEvent() apiparams.AuditEvent {
	var ale apiparams.AuditEvent
	ale.Time = e.Time
	ale.ConversationId = e.ConversationId
	ale.MessageId = e.MessageId
	ale.FacadeMethod = e.FacadeMethod
	ale.FacadeName = e.FacadeName
	ale.FacadeVersion = e.FacadeVersion
	ale.ObjectId = e.ObjectId
	ale.UserTag = e.IdentityTag
	ale.Model = e.Model
	ale.IsResponse = e.IsResponse
	ale.Errors = nil
	if e.IsResponse {
		err := json.Unmarshal(e.Errors, &ale.Errors)
		if err != nil {
			ale.Errors = map[string]any{"error": err}
		}
	}
	if e.Params != nil {
		err := json.Unmarshal(e.Params, &ale.Params)
		if err != nil {
			ale.Params = map[string]any{"error": err}
		}
	}
	return ale
}
