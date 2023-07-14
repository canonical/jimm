// Copyright 2020 Canonical Ltd.

package dbmodel_test

import (
	"testing"
	"time"

	qt "github.com/frankban/quicktest"
	"github.com/juju/names/v4"

	apiparams "github.com/canonical/jimm/api/params"
	"github.com/canonical/jimm/internal/dbmodel"
)

func TestAuditLogEntry(t *testing.T) {
	c := qt.New(t)
	db := gormDB(t)

	ale := dbmodel.AuditLogEntry{
		Time:    time.Now(),
		Tag:     names.NewModelTag("00000001-0000-0000-0000-0000-000000000008").String(),
		UserTag: names.NewUserTag("bob@external").String(),
		Action:  "created",
		Success: true,
		Params:  dbmodel.StringMap{"a": "b", "c": "d"},
	}
	c.Assert(db.Create(&ale).Error, qt.IsNil)

	var ale2 dbmodel.AuditLogEntry
	c.Assert(db.First(&ale2).Error, qt.IsNil)
	c.Check(ale2, qt.DeepEquals, ale)
}

func TestToAPIAuditEvent(t *testing.T) {
	c := qt.New(t)

	ale := dbmodel.AuditLogEntry{
		Time:    time.Now(),
		Tag:     names.NewModelTag("00000001-0000-0000-0000-0000-000000000008").String(),
		UserTag: names.NewUserTag("bob@external").String(),
		Action:  "created",
		Success: true,
		Params:  dbmodel.StringMap{"a": "b", "c": "d"},
	}
	event := ale.ToAPIAuditEvent()
	c.Check(event, qt.DeepEquals, apiparams.AuditEvent{
		Time:    ale.Time,
		Tag:     names.NewModelTag("00000001-0000-0000-0000-0000-000000000008").String(),
		UserTag: names.NewUserTag("bob@external").String(),
		Action:  "created",
		Success: true,
		Params: map[string]string{
			"a": "b",
			"c": "d",
		},
	})

	ale.Success = false
	event = ale.ToAPIAuditEvent()
	c.Check(event, qt.DeepEquals, apiparams.AuditEvent{
		Time:    ale.Time,
		Tag:     names.NewModelTag("00000001-0000-0000-0000-0000-000000000008").String(),
		UserTag: names.NewUserTag("bob@external").String(),
		Action:  "created",
		Success: false,
		Params: map[string]string{
			"a": "b",
			"c": "d",
		},
	})
}
