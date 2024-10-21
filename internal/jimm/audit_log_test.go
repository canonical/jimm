// Copyright 2024 Canonical.

package jimm_test

import (
	"context"
	"testing"
	"time"

	qt "github.com/frankban/quicktest"

	"github.com/canonical/jimm/v3/internal/db"
	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/jimm"
	"github.com/canonical/jimm/v3/internal/testutils/jimmtest"
)

func TestAuditLogCleanupServicePurgesLogs(t *testing.T) {
	c := qt.New(t)

	ctx := context.Background()
	now := time.Now().UTC().Round(time.Millisecond)

	db := db.Database{
		DB: jimmtest.PostgresDB(c, func() time.Time { return now }),
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
	svc := jimm.NewAuditLogCleanupService(db, 1)
	svc.Start(ctx)

	// Check 2 were purged
	logs = make([]dbmodel.AuditLogEntry, 0)
	err = db.DB.Find(&logs).Error
	c.Assert(err, qt.IsNil)
	c.Assert(logs, qt.HasLen, 3)
}

func TestCalculateNextPollDuration(t *testing.T) {
	c := qt.New(t)

	// Test where 9am is behind 12pm
	startingTime := time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC)
	d := jimm.CalculateNextPollDuration(startingTime)
	c.Assert(d, qt.Equals, time.Hour*21)

	// Test where 9am is ahead of 7pm
	startingTime = time.Date(2023, 1, 1, 7, 0, 0, 0, time.UTC)
	d = jimm.CalculateNextPollDuration(startingTime)
	c.Assert(d, qt.Equals, time.Hour*2)
}
