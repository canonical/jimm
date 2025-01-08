// Copyright 2025 Canonical.
package auditlog_test

import (
	"context"
	"testing"
	"time"

	qt "github.com/frankban/quicktest"

	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/jimm/auditlog"
)

func (s *auditLogManagerSuite) TestAuditLogCleanupServicePurgesLogs(c *qt.C) {
	c.Parallel()
	ctx := context.Background()
	now := time.Now().UTC().Round(time.Millisecond)

	err := s.db.AddAuditLogEntry(ctx, &dbmodel.AuditLogEntry{
		Time: now.AddDate(0, 0, -1),
	})
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeUpgradeInProgress)

	// A log from 1 day ago
	c.Assert(s.db.AddAuditLogEntry(ctx, &dbmodel.AuditLogEntry{
		Time: now.AddDate(0, 0, -1),
	}), qt.IsNil)

	// A log from 2 days ago
	c.Assert(s.db.AddAuditLogEntry(ctx, &dbmodel.AuditLogEntry{
		Time: now.AddDate(0, 0, -2),
	}), qt.IsNil)

	// A log from 3 days ago
	c.Assert(s.db.AddAuditLogEntry(ctx, &dbmodel.AuditLogEntry{
		Time: now.AddDate(0, 0, -3),
	}), qt.IsNil)

	// Check 3 created
	logs := make([]dbmodel.AuditLogEntry, 0)
	err = s.db.DB.Find(&logs).Error
	c.Assert(err, qt.IsNil)
	c.Assert(logs, qt.HasLen, 3)

	auditlog.PollDuration.Hours = now.Hour()
	auditlog.PollDuration.Minutes = now.Minute()
	auditlog.PollDuration.Seconds = now.Second() + 2
	// Suite is setup to clean logs more than 1 day old.
	s.manager.StartCleanup(ctx)

	// Check 2 were purged
	logs = make([]dbmodel.AuditLogEntry, 0)
	err = s.db.DB.Find(&logs).Error
	c.Assert(err, qt.IsNil)
	c.Assert(logs, qt.HasLen, 3)
}

func TestCalculateNextPollDuration(t *testing.T) {
	c := qt.New(t)

	// Test where 9am is behind 12pm
	startingTime := time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC)
	d := auditlog.CalculateNextPollDuration(startingTime)
	c.Assert(d, qt.Equals, time.Hour*21)

	// Test where 9am is ahead of 7pm
	startingTime = time.Date(2023, 1, 1, 7, 0, 0, 0, time.UTC)
	d = auditlog.CalculateNextPollDuration(startingTime)
	c.Assert(d, qt.Equals, time.Hour*2)
}
