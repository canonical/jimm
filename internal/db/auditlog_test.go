// Copyright 2021 Canonical Ltd.

package db_test

import (
	"context"
	"testing"
	"time"

	qt "github.com/frankban/quicktest"
	"github.com/juju/names/v5"

	"github.com/canonical/jimm/v3/internal/db"
	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
)

func TestAddAuditLogEntryUnconfiguredDatabase(t *testing.T) {
	c := qt.New(t)

	var d db.Database
	err := d.AddAuditLogEntry(context.Background(), &dbmodel.AuditLogEntry{})
	c.Check(err, qt.ErrorMatches, `database not configured`)
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeServerConfiguration)
}

func (s *dbSuite) TestAddAuditLogEntry(c *qt.C) {
	ctx := context.Background()

	ale := dbmodel.AuditLogEntry{
		Time:        time.Now().UTC().Round(time.Millisecond),
		IdentityTag: names.NewUserTag("alice@canonical.com").String(),
	}

	err := s.Database.AddAuditLogEntry(ctx, &ale)
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeUpgradeInProgress)

	err = s.Database.Migrate(context.Background(), false)
	c.Assert(err, qt.IsNil)

	err = s.Database.AddAuditLogEntry(ctx, &ale)
	c.Assert(err, qt.IsNil)

	var ale2 dbmodel.AuditLogEntry
	err = s.Database.ForEachAuditLogEntry(ctx, db.AuditLogFilter{}, func(ale *dbmodel.AuditLogEntry) error {
		if ale2.ID != 0 {
			return errors.E("too many results")
		}
		ale2 = *ale
		return nil
	})
	c.Assert(err, qt.IsNil)
	c.Check(ale2, qt.DeepEquals, ale)
}

func TestForEachAuditLogEntryUnconfiguredDatabase(t *testing.T) {
	c := qt.New(t)

	var d db.Database
	err := d.ForEachAuditLogEntry(context.Background(), db.AuditLogFilter{}, nil)
	c.Check(err, qt.ErrorMatches, `database not configured`)
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeServerConfiguration)
}

var testAuditLogEntries = []dbmodel.AuditLogEntry{{
	Time:        time.Date(2020, time.February, 20, 20, 2, 20, 0, time.UTC),
	IdentityTag: names.NewUserTag("alice@canonical.com").String(),
}, {
	Time:        time.Date(2020, time.February, 20, 20, 2, 21, 0, time.UTC),
	IdentityTag: names.NewUserTag("alice@canonical.com").String(),
}, {
	Time:        time.Date(2020, time.February, 20, 20, 2, 21, 0, time.UTC),
	IdentityTag: names.NewUserTag("bob@canonical.com").String(),
}, {
	Time:        time.Date(2020, time.February, 20, 20, 2, 23, 0, time.UTC),
	IdentityTag: names.NewUserTag("alice@canonical.com").String(),
}}

var forEachAuditLogEntryTests = []struct {
	name          string
	filter        db.AuditLogFilter
	expectEntries []int
}{{
	name:          "NoFilter",
	filter:        db.AuditLogFilter{},
	expectEntries: []int{0, 1, 2, 3},
}, {
	name: "StartFilter",
	filter: db.AuditLogFilter{
		Start: time.Date(2020, time.February, 20, 20, 2, 21, 0, time.UTC),
	},
	expectEntries: []int{1, 2, 3},
}, {
	name: "EndFilter",
	filter: db.AuditLogFilter{
		End: time.Date(2020, time.February, 20, 20, 2, 22, 0, time.UTC),
	},
	expectEntries: []int{0, 1, 2},
}, {
	name: "RangeFilter",
	filter: db.AuditLogFilter{
		Start: time.Date(2020, time.February, 20, 20, 2, 21, 0, time.UTC),
		End:   time.Date(2020, time.February, 20, 20, 2, 22, 0, time.UTC),
	},
	expectEntries: []int{1, 2},
}, {
	name: "UserTagFilter",
	filter: db.AuditLogFilter{
		IdentityTag: names.NewUserTag("alice@canonical.com").String(),
	},
	expectEntries: []int{0, 1, 3},
}}

func (s *dbSuite) TestForEachAuditLogEntry(c *qt.C) {
	ctx := context.Background()

	err := s.Database.ForEachAuditLogEntry(context.Background(), db.AuditLogFilter{}, nil)
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeUpgradeInProgress)

	err = s.Database.Migrate(context.Background(), false)
	c.Assert(err, qt.IsNil)

	for i := range testAuditLogEntries {
		err := s.Database.AddAuditLogEntry(ctx, &testAuditLogEntries[i])
		c.Assert(err, qt.IsNil)
	}

	for _, test := range forEachAuditLogEntryTests {
		c.Run(test.name, func(c *qt.C) {
			var ales []dbmodel.AuditLogEntry
			err := s.Database.ForEachAuditLogEntry(ctx, test.filter, func(ale *dbmodel.AuditLogEntry) error {
				ales = append(ales, *ale)
				return nil
			})
			c.Assert(err, qt.IsNil)
			c.Assert(ales, qt.HasLen, len(test.expectEntries))
			for i := range ales {
				c.Check(ales[i], qt.DeepEquals, testAuditLogEntries[test.expectEntries[i]])
			}
		})
	}

	var calls int
	testError := errors.E("a test error")
	err = s.Database.ForEachAuditLogEntry(context.Background(), db.AuditLogFilter{}, func(_ *dbmodel.AuditLogEntry) error {
		calls++
		return testError
	})
	c.Check(calls, qt.Equals, 1)
	c.Check(err, qt.DeepEquals, testError)
}

func (s *dbSuite) TestDeleteAuditLogsBefore(c *qt.C) {
	ctx := context.Background()
	now := time.Now()

	err := s.Database.AddAuditLogEntry(ctx, &dbmodel.AuditLogEntry{
		Time: now.AddDate(0, 0, -1),
	})
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeUpgradeInProgress)

	err = s.Database.Migrate(context.Background(), true)
	c.Assert(err, qt.IsNil)

	// Delete all when none exist
	retentionDate := time.Now()
	deleted, err := s.Database.DeleteAuditLogsBefore(ctx, retentionDate)
	c.Assert(err, qt.IsNil)
	c.Assert(deleted, qt.Equals, int64(0))

	// A log from 1 day ago
	c.Assert(s.Database.AddAuditLogEntry(ctx, &dbmodel.AuditLogEntry{
		Time: now.AddDate(0, 0, -1),
	}), qt.IsNil)

	// A log from 2 days ago
	c.Assert(s.Database.AddAuditLogEntry(ctx, &dbmodel.AuditLogEntry{
		Time: now.AddDate(0, 0, -2),
	}), qt.IsNil)

	// A log from 3 days ago
	c.Assert(s.Database.AddAuditLogEntry(ctx, &dbmodel.AuditLogEntry{
		Time: now.AddDate(0, 0, -3),
	}), qt.IsNil)

	// Ensure 3 exist
	logs := make([]dbmodel.AuditLogEntry, 0)
	err = s.Database.DB.Find(&logs).Error
	c.Assert(err, qt.IsNil)
	c.Assert(logs, qt.HasLen, 3)

	// Delete all 2 or more days older, leaving 1 log left
	retentionDate = time.Now().AddDate(0, 0, -(2))
	deleted, err = s.Database.DeleteAuditLogsBefore(ctx, retentionDate)
	c.Assert(err, qt.IsNil)

	// Check that 2 were infact deleted
	c.Assert(deleted, qt.Equals, int64(2))

	// Check only 1 remains
	logs = make([]dbmodel.AuditLogEntry, 0)
	err = s.Database.DB.Find(&logs).Error
	c.Assert(err, qt.IsNil)
	c.Assert(logs, qt.HasLen, 1)
}

func (s *dbSuite) TestPurgeLogsFromDb(c *qt.C) {

	ctx := context.Background()
	relativeNow := time.Now().AddDate(-1, 0, 0)
	ale := dbmodel.AuditLogEntry{
		Time:        relativeNow.UTC().Round(time.Millisecond),
		IdentityTag: names.NewUserTag("alice@canonical.com").String(),
	}
	ale_past := dbmodel.AuditLogEntry{
		Time:        relativeNow.AddDate(0, 0, -1).UTC().Round(time.Millisecond),
		IdentityTag: names.NewUserTag("alice@canonical.com").String(),
	}
	ale_future := dbmodel.AuditLogEntry{
		Time:        relativeNow.AddDate(0, 0, 5).UTC().Round(time.Millisecond),
		IdentityTag: names.NewUserTag("alice@canonical.com").String(),
	}

	err := s.Database.Migrate(context.Background(), false)
	c.Assert(err, qt.IsNil)
	err = s.Database.AddAuditLogEntry(ctx, &ale)
	c.Assert(err, qt.IsNil)
	err = s.Database.AddAuditLogEntry(ctx, &ale_past)
	c.Assert(err, qt.IsNil)
	err = s.Database.AddAuditLogEntry(ctx, &ale_future)
	c.Assert(err, qt.IsNil)
	deleted_count, err := s.Database.DeleteAuditLogsBefore(ctx, relativeNow.AddDate(0, 0, 1))
	// check that logs have been deleted
	c.Assert(err, qt.IsNil)
	c.Assert(deleted_count, qt.Equals, int64(2))
}
