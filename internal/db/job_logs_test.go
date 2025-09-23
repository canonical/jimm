// Copyright 2025 Canonical.

package db_test

import (
	qt "github.com/frankban/quicktest"
	"github.com/google/uuid"

	"github.com/canonical/jimm/v3/internal/db"
	"github.com/canonical/jimm/v3/internal/dbmodel"
)

func (s *dbSuite) TestJobLogs_AddJobLog(c *qt.C) {
	ctx := c.Context()

	err := s.Database.Migrate(ctx)
	c.Assert(err, qt.IsNil)

	// Add job to reference
	jobId, err := s.Database.AddJob(ctx, "test-job")
	c.Assert(err, qt.IsNil)

	// Test where job id doesn't exist
	jobThatDoesntExistId := uuid.New()
	err = s.Database.AddJobLog(ctx, jobThatDoesntExistId, "Creating Juju controller \"diglett\" on the-most-amazing-cloud")
	c.Assert(err, qt.ErrorMatches, ".*violates foreign key constraint.*")

	// Test success
	err = s.Database.AddJobLog(ctx, jobId, "Creating Juju controller \"diglett\" on the-most-amazing-cloud")
	c.Assert(err, qt.IsNil)
	// Test adding second line
	err = s.Database.AddJobLog(ctx, jobId, "Fetching Juju agent binaries")
	c.Assert(err, qt.IsNil)
	// Check all lines exist
	var logs []dbmodel.JobLog
	err = s.Database.DB.Where("job_id = ?", jobId).Order("line_number asc").Find(&logs).Error
	c.Assert(err, qt.IsNil)

	c.Assert(logs, qt.HasLen, 2)
	c.Assert(logs[0].LineNumber, qt.Equals, 1)
	c.Assert(logs[0].LogLine, qt.Equals, "Creating Juju controller \"diglett\" on the-most-amazing-cloud")
	c.Assert(logs[1].LineNumber, qt.Equals, 2)
	c.Assert(logs[1].LogLine, qt.Equals, "Fetching Juju agent binaries")

	// Test adding another where job id is different
	jobId2, err := s.Database.AddJob(ctx, "test-job")
	c.Assert(err, qt.IsNil)
	err = s.Database.AddJobLog(ctx, jobId2, "Creating Juju controller \"diglett2\" on the-most-amazing-cloud")
	c.Assert(err, qt.IsNil)

	var logs2 []dbmodel.JobLog
	err = s.Database.DB.Where("job_id = ?", jobId2).Order("line_number asc").Find(&logs2).Error
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
	jobIdThatDoesntExist := uuid.New()
	_, _, err = s.Database.QueryJobLog(ctx, jobIdThatDoesntExist, 0)
	c.Assert(err, qt.ErrorMatches, "job not found")

	// Add job to reference
	jobId, err := s.Database.AddJob(ctx, "test-job")
	c.Assert(err, qt.IsNil)

	// Query with no logs
	loggies, nextOffsetVal, err := s.Database.QueryJobLog(ctx, jobId, 0)
	c.Assert(err, qt.IsNil)
	c.Assert(loggies, qt.HasLen, 0)
	c.Assert(nextOffsetVal, qt.Equals, 0)

	// Test iterating through a simulated incoming logs
	offsetTracker := 0
	collectedLogs := make([]string, 0)

	newLogs, nextOffsetValue, err := s.Database.QueryJobLog(ctx, jobId, offsetTracker)
	c.Assert(err, qt.IsNil)
	offsetTracker = nextOffsetValue
	collectedLogs = append(collectedLogs, newLogs...)

	c.Assert(s.Database.AddJobLog(ctx, jobId, "Creating Juju controller \"diglett\" on the-most-amazing-cloud"), qt.IsNil)
	c.Assert(s.Database.AddJobLog(ctx, jobId, "Fetching Juju agent binaries"), qt.IsNil)

	newLogs, nextOffsetValue, err = s.Database.QueryJobLog(ctx, jobId, offsetTracker)
	c.Assert(err, qt.IsNil)
	offsetTracker = nextOffsetValue
	collectedLogs = append(collectedLogs, newLogs...)

	c.Assert(s.Database.AddJobLog(ctx, jobId, "Binaries contain gems"), qt.IsNil)
	c.Assert(s.Database.AddJobLog(ctx, jobId, "Gems appear to be very expensive"), qt.IsNil)

	newLogs, nextOffsetValue, err = s.Database.QueryJobLog(ctx, jobId, offsetTracker)
	c.Assert(err, qt.IsNil)
	offsetTracker = nextOffsetValue
	collectedLogs = append(collectedLogs, newLogs...)

	c.Assert(collectedLogs, qt.HasLen, 4)

	newLogs, nextOffsetValue, err = s.Database.QueryJobLog(ctx, jobId, offsetTracker)
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

	jobId, err := s.Database.AddJob(ctx, "test-job")
	c.Assert(err, qt.IsNil)

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

	err = s.Database.AddJobLog(ctx, jobId, "Creating Juju controller \"diglett\" on the-most-amazing-cloud")
	c.Assert(err, qt.ErrorMatches, "failed to lock job_logs table")
}
