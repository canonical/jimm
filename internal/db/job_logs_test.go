// Copyright 2025 Canonical.

package db_test

import (
	qt "github.com/frankban/quicktest"

	"github.com/canonical/jimm/v3/internal/db"
	"github.com/canonical/jimm/v3/internal/dbmodel"
)

func (s *dbSuite) TestJobLogs_AddJobLog(c *qt.C) {
	ctx := c.Context()

	err := s.Database.Migrate(ctx)
	c.Assert(err, qt.IsNil)

	jobID := int64(1)

	// Test success
	err = s.Database.AddJobLog(ctx, jobID, "Creating Juju controller \"diglett\" on the-most-amazing-cloud")
	c.Assert(err, qt.IsNil)
	// Test adding second line
	err = s.Database.AddJobLog(ctx, jobID, "Fetching Juju agent binaries")
	c.Assert(err, qt.IsNil)
	// Check all lines exist
	var logs []dbmodel.JobLog
	err = s.Database.DB.Where("job_id = ?", jobID).Order("line_number asc").Find(&logs).Error
	c.Assert(err, qt.IsNil)

	c.Assert(logs, qt.HasLen, 2)
	c.Assert(logs[0].LineNumber, qt.Equals, 1)
	c.Assert(logs[0].LogLine, qt.Equals, "Creating Juju controller \"diglett\" on the-most-amazing-cloud")
	c.Assert(logs[1].LineNumber, qt.Equals, 2)
	c.Assert(logs[1].LogLine, qt.Equals, "Fetching Juju agent binaries")

	// Test adding another where job id is different
	jobID2 := int64(2)
	err = s.Database.AddJobLog(ctx, jobID2, "Creating Juju controller \"diglett2\" on the-most-amazing-cloud")
	c.Assert(err, qt.IsNil)

	var logs2 []dbmodel.JobLog
	err = s.Database.DB.Where("job_id = ?", jobID2).Order("line_number asc").Find(&logs2).Error
	c.Assert(err, qt.IsNil)

	c.Assert(logs2, qt.HasLen, 1)
	c.Assert(logs2[0].LineNumber, qt.Equals, 1)
	c.Assert(logs2[0].LogLine, qt.Equals, "Creating Juju controller \"diglett2\" on the-most-amazing-cloud")
}

func (s *dbSuite) TestJobLogs_QueryJobLogs(c *qt.C) {
	ctx := c.Context()

	err := s.Database.Migrate(ctx)
	c.Assert(err, qt.IsNil)

	// Query where the job doesn't exist
	logs, _, err := s.Database.QueryJobLog(ctx, 0, 0)
	c.Assert(err, qt.IsNil)
	c.Assert(logs, qt.HasLen, 0)

	// Add job to reference
	jobID := int64(1)

	// Query with no logs
	loggies, nextOffsetVal, err := s.Database.QueryJobLog(ctx, jobID, 0)
	c.Assert(err, qt.IsNil)
	c.Assert(loggies, qt.HasLen, 0)
	c.Assert(nextOffsetVal, qt.Equals, 0)

	// Test iterating through a simulated incoming logs
	offsetTracker := 0
	collectedLogs := make([]string, 0)

	newLogs, nextOffsetValue, err := s.Database.QueryJobLog(ctx, jobID, offsetTracker)
	c.Assert(err, qt.IsNil)
	offsetTracker = nextOffsetValue
	collectedLogs = append(collectedLogs, newLogs...)

	c.Assert(s.Database.AddJobLog(ctx, jobID, "Creating Juju controller \"diglett\" on the-most-amazing-cloud"), qt.IsNil)
	c.Assert(s.Database.AddJobLog(ctx, jobID, "Fetching Juju agent binaries"), qt.IsNil)

	newLogs, nextOffsetValue, err = s.Database.QueryJobLog(ctx, jobID, offsetTracker)
	c.Assert(err, qt.IsNil)
	offsetTracker = nextOffsetValue
	collectedLogs = append(collectedLogs, newLogs...)

	c.Assert(s.Database.AddJobLog(ctx, jobID, "Binaries contain gems"), qt.IsNil)
	c.Assert(s.Database.AddJobLog(ctx, jobID, "Gems appear to be very expensive"), qt.IsNil)

	newLogs, nextOffsetValue, err = s.Database.QueryJobLog(ctx, jobID, offsetTracker)
	c.Assert(err, qt.IsNil)
	offsetTracker = nextOffsetValue
	collectedLogs = append(collectedLogs, newLogs...)

	c.Assert(collectedLogs, qt.HasLen, 4)

	newLogs, nextOffsetValue, err = s.Database.QueryJobLog(ctx, jobID, offsetTracker)
	c.Assert(err, qt.IsNil)
	// This means no new logs have come in, but they may later, and the client should query again for logs
	// after some time.
	c.Assert(newLogs, qt.HasLen, 0)
	c.Assert(nextOffsetValue, qt.Equals, offsetTracker)
}

// This test is a behaviour check, that is, we want our lock does indeed reject
// on an ACCESS EXCLUSIVE mode when inserting new logs.
func (s *dbSuite) TestJobLogs_lockJobLogs(c *qt.C) {
	ctx := c.Context()

	err := s.Database.Migrate(ctx)
	c.Assert(err, qt.IsNil)

	jobID := int64(1)

	finishTransaction := make(chan bool)
	c.Cleanup(func() {
		close(finishTransaction)
	})
	lockAcquired := make(chan bool)

	// Adjust the query to use NOWAIT, such that it can error immediately
	// within our AddJobLog call. The routine below will successfully
	// acquire a lock because none is present yet. When we attempt to acquire
	// it again in our AddJobLogs call, it is going to immediately error
	// and not queue.
	c.Patch(db.JobLogLockQuery, *db.JobLogLockQuery+" NOWAIT")
	go func() {
		err := s.Database.Transaction(func(d *db.Database) error {
			err := d.DB.Exec(*db.JobLogLockQuery).Error
			if err != nil {
				return err
			}

			close(lockAcquired)

			<-finishTransaction
			return nil
		})
		c.Assert(err, qt.IsNil)
	}()

	<-lockAcquired

	err = s.Database.AddJobLog(ctx, jobID, "Creating Juju controller \"diglett\" on the-most-amazing-cloud")
	c.Assert(err, qt.ErrorMatches, "failed to lock job_logs table")
}
