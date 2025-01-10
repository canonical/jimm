// Copyright 2025 Canonical.

package auditlog_test

import (
	"context"
	"testing"
	"time"

	qt "github.com/frankban/quicktest"
	"github.com/juju/names/v5"

	"github.com/canonical/jimm/v3/internal/db"
	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/jimm/auditlog"
	"github.com/canonical/jimm/v3/internal/testutils/jimmtest"
)

func TestAuditLogCleanupServicePurgesLogs(t *testing.T) {
	c := qt.New(t)
	c.Parallel()

	ctx := context.Background()

	db := &db.Database{
		DB: jimmtest.PostgresDB(c, time.Now),
	}
	err := db.Migrate(context.Background())
	c.Assert(err, qt.IsNil)

	ofgaClient, _, _, err := jimmtest.SetupTestOFGAClient(c.Name())
	c.Assert(err, qt.IsNil)

	jimmTag := names.NewControllerTag("foo")

	manager, err := auditlog.NewAuditLogManager(db, ofgaClient, jimmTag, 1)
	c.Assert(err, qt.IsNil)

	now := time.Now().UTC()

	// A log from today
	c.Assert(db.AddAuditLogEntry(ctx, &dbmodel.AuditLogEntry{
		Time: now.AddDate(0, 0, 0),
	}), qt.IsNil)

	// A log from 1 day ago
	c.Assert(db.AddAuditLogEntry(ctx, &dbmodel.AuditLogEntry{
		Time: now.AddDate(0, 0, -1),
	}), qt.IsNil)

	// A log from 2 days ago
	c.Assert(db.AddAuditLogEntry(ctx, &dbmodel.AuditLogEntry{
		Time: now.AddDate(0, 0, -2),
	}), qt.IsNil)

	// Check 3 created
	logs := make([]dbmodel.AuditLogEntry, 0)
	err = db.DB.Find(&logs).Error
	c.Assert(err, qt.IsNil)
	c.Assert(logs, qt.HasLen, 3)

	// Manager is setup above to remove logs older than 1 day.
	manager.Cleanup(ctx)

	// Check 2 were purged
	logs = make([]dbmodel.AuditLogEntry, 0)
	err = db.DB.Find(&logs).Error
	c.Assert(err, qt.IsNil)
	c.Assert(logs, qt.HasLen, 1)
}

func TestCalculateNextPollDuration(t *testing.T) {
	c := qt.New(t)

	pollTime := auditlog.PollTimeOfDay{Hours: 9}

	// Test where 9am is behind 12pm
	startingTime := time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC)
	d := auditlog.CalculateNextPollDuration(pollTime, startingTime)
	c.Assert(d, qt.Equals, time.Hour*21)

	// Test where 9am is ahead of 7am
	startingTime = time.Date(2023, 1, 1, 7, 0, 0, 0, time.UTC)
	d = auditlog.CalculateNextPollDuration(pollTime, startingTime)
	c.Assert(d, qt.Equals, time.Hour*2)
}
