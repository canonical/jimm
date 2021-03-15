// Copyright 2020 Canonical Ltd.

package dbmodel_test

import (
	"testing"
	"time"

	qt "github.com/frankban/quicktest"
	"github.com/juju/names/v4"

	"github.com/CanonicalLtd/jimm/internal/dbmodel"
)

func TestAuditLogEntry(t *testing.T) {
	c := qt.New(t)
	db := gormDB(t, &dbmodel.AuditLogEntry{})

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
