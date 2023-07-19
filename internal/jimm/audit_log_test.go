// Copyright 2023 Canonical Ltd.

package jimm_test

import (
	"context"
	"testing"
	"time"

	"github.com/CanonicalLtd/jimm/internal/db"
	"github.com/CanonicalLtd/jimm/internal/dbmodel"
	"github.com/CanonicalLtd/jimm/internal/errors"
	"github.com/CanonicalLtd/jimm/internal/jimm"
	"github.com/CanonicalLtd/jimm/internal/jimmtest"
	qt "github.com/frankban/quicktest"
)

func TestAuditLogCleanupServicePurgesLogs(t *testing.T) {
	c := qt.New(t)

	ctx := context.Background()
	now := time.Now().UTC().Round(time.Millisecond)

	db := db.Database{
		DB: jimmtest.MemoryDB(c, func() time.Time { return now }),
	}

	err := db.AddAuditLogEntry(ctx, &dbmodel.AuditLogEntry{
		Time: now.AddDate(0, 0, -1),
	})
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeUpgradeInProgress)

	err = db.Migrate(context.Background(), true)
	c.Assert(err, qt.IsNil)

	// A log from 1 day ago
	c.Assert(db.AddAuditLogEntry(ctx, &dbmodel.AuditLogEntry{
		Time: now.AddDate(0, 0, -1),
	}), qt.IsNil)

	// A log from 2 days ago
	c.Assert(db.AddAuditLogEntry(ctx, &dbmodel.AuditLogEntry{
		Time: now.AddDate(0, 0, -2),
	}), qt.IsNil)

	// A log from 3 days ago
	c.Assert(db.AddAuditLogEntry(ctx, &dbmodel.AuditLogEntry{
		Time: now.AddDate(0, 0, -3),
	}), qt.IsNil)

	// Check 3 created
	logs := make([]dbmodel.AuditLogEntry, 0)
	err = db.DB.Find(&logs).Error
	c.Assert(err, qt.IsNil)
	c.Assert(logs, qt.HasLen, 3)

	jimm.PollDuration.Hours = now.Hour()
	jimm.PollDuration.Minutes = now.Minute()
	jimm.PollDuration.Seconds = now.Second() + 2
	svc := jimm.NewAuditLogCleanupService(ctx, db, 1)
	svc.Start()

	// Check 2 were purged
	logs = make([]dbmodel.AuditLogEntry, 0)
	err = db.DB.Find(&logs).Error
	c.Assert(err, qt.IsNil)
	c.Assert(logs, qt.HasLen, 3)
}
