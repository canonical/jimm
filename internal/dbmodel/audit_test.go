// Copyright 2020 Canonical Ltd.

package dbmodel_test

import (
	"encoding/json"
	"testing"
	"time"

	qt "github.com/frankban/quicktest"
	"github.com/juju/names/v5"

	apiparams "github.com/canonical/jimm/api/params"
	"github.com/canonical/jimm/internal/dbmodel"
)

func TestAuditLogEntry(t *testing.T) {
	c := qt.New(t)
	db := gormDB(t)

	params := map[string]any{"a": "b", "c": "d"}
	paramsJSON, err := json.Marshal(params)
	c.Assert(err, qt.IsNil)

	ale := dbmodel.AuditLogEntry{
		Time:           time.Now().Truncate(time.Second),
		ConversationId: "1234",
		MessageId:      9876,
		FacadeName:     "JIMM",
		FacadeMethod:   "AddController",
		FacadeVersion:  1,
		ObjectId:       "1",
		IdentityTag:    names.NewUserTag("bob@external").String(),
		IsResponse:     false,
		Params:         paramsJSON,
		Errors:         nil,
	}
	c.Assert(db.Create(&ale).Error, qt.IsNil)

	var ale2 dbmodel.AuditLogEntry
	c.Assert(db.First(&ale2).Error, qt.IsNil)
	c.Check(ale2, qt.DeepEquals, ale)
}

func TestToAPIAuditEvent(t *testing.T) {
	c := qt.New(t)

	params := map[string]any{"a": "b", "c": "d"}
	paramsJSON, err := json.Marshal(params)
	c.Assert(err, qt.IsNil)

	ale := dbmodel.AuditLogEntry{
		Time:           time.Now().Truncate(time.Second),
		ConversationId: "1234",
		MessageId:      9876,
		FacadeName:     "JIMM",
		FacadeMethod:   "AddController",
		FacadeVersion:  1,
		ObjectId:       "1",
		IdentityTag:    names.NewUserTag("bob@external").String(),
		IsResponse:     false,
		Params:         paramsJSON,
		Errors:         nil,
	}
	event := ale.ToAPIAuditEvent()
	expectedEvent := apiparams.AuditEvent{
		Time:           ale.Time,
		ConversationId: "1234",
		MessageId:      9876,
		FacadeName:     "JIMM",
		FacadeMethod:   "AddController",
		FacadeVersion:  1,
		ObjectId:       "1",
		UserTag:        names.NewUserTag("bob@external").String(),
		IsResponse:     false,
		Params:         map[string]any{"a": "b", "c": "d"},
		Errors:         nil,
	}
	c.Check(event, qt.DeepEquals, expectedEvent)

	// And test with a response
	errors := map[string]any{}
	errorsJSON, err := json.Marshal(errors)
	c.Assert(err, qt.IsNil)
	ale.Errors = errorsJSON
	ale.IsResponse = true
	event = ale.ToAPIAuditEvent()
	expectedEvent.IsResponse = true
	expectedEvent.Errors = map[string]any{}
	c.Check(event, qt.DeepEquals, expectedEvent)
}
