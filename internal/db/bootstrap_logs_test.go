// Copyright 2025 Canonical.

package db_test

import (
	"time"

	qt "github.com/frankban/quicktest"
	"github.com/google/uuid"

	"github.com/canonical/jimm/v3/internal/db"
	"github.com/canonical/jimm/v3/internal/dbmodel"
)

func (s *dbSuite) TestBootstrapLogs_AddBootstrapLog(c *qt.C) {
	ctx := c.Context()

	err := s.Database.Migrate(ctx)
	c.Assert(err, qt.IsNil)

	// Add job to reference
	jobId, err := s.Database.AddJob(ctx, "test-job")
	c.Assert(err, qt.IsNil)

	// Test where job id doesn't exist
	jobThatDoesntExistId := uuid.New()
	err = s.Database.AddBootstrapLog(ctx, jobThatDoesntExistId, "Creating Juju controller \"diglett\" on the-most-amazing-cloud")
	c.Assert(err, qt.ErrorMatches, ".*violates foreign key constraint.*")

	// Test success
	err = s.Database.AddBootstrapLog(ctx, jobId, "Creating Juju controller \"diglett\" on the-most-amazing-cloud")
	c.Assert(err, qt.IsNil)
	// Test adding second line
	err = s.Database.AddBootstrapLog(ctx, jobId, "Fetching Juju agent binaries")
	c.Assert(err, qt.IsNil)
	// Check all lines exist
	var logs []dbmodel.BootstrapLog
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
	err = s.Database.AddBootstrapLog(ctx, jobId2, "Creating Juju controller \"diglett2\" on the-most-amazing-cloud")
	c.Assert(err, qt.IsNil)

	var logs2 []dbmodel.BootstrapLog
	err = s.Database.DB.Where("job_id = ?", jobId2).Order("line_number asc").Find(&logs2).Error
	c.Assert(err, qt.IsNil)

	c.Assert(logs2, qt.HasLen, 1)
	c.Assert(logs2[0].LineNumber, qt.Equals, 1)
	c.Assert(logs2[0].LogLine, qt.Equals, "Creating Juju controller \"diglett2\" on the-most-amazing-cloud")
}

func (s *dbSuite) TestBootstrapLogs_QueryBootstrapLogs(c *qt.C) {
	ctx := c.Context()

	err := s.Database.Migrate(ctx)
	c.Assert(err, qt.IsNil)

	// Query where the job doesn't exist
	jobIdThatDoesntExist := uuid.New()
	_, err = s.Database.QueryBootstrapLog(ctx, jobIdThatDoesntExist, 0)
	c.Assert(err, qt.ErrorMatches, "job not found")

	// Add job to reference
	jobId, err := s.Database.AddJob(ctx, "test-job")
	c.Assert(err, qt.IsNil)

	// Query with no logs
	_, err = s.Database.QueryBootstrapLog(ctx, jobId, 0)
	c.Assert(err, qt.ErrorMatches, "not found")
	// Query with one log
	err = s.Database.AddBootstrapLog(ctx, jobId, "Creating Juju controller \"diglett\" on the-most-amazing-cloud")
	c.Assert(err, qt.IsNil)

	loggies, err := s.Database.QueryBootstrapLog(ctx, jobId, 0)
	c.Assert(err, qt.IsNil)
	c.Assert(loggies[0], qt.Equals, "Creating Juju controller \"diglett\" on the-most-amazing-cloud")

	// Query with two logs, offset 0
	err = s.Database.AddBootstrapLog(ctx, jobId, "Fetching Juju agent binaries")
	c.Assert(err, qt.IsNil)

	loggies, err = s.Database.QueryBootstrapLog(ctx, jobId, 0)
	c.Assert(err, qt.IsNil)
	c.Assert(loggies[0], qt.Equals, "Creating Juju controller \"diglett\" on the-most-amazing-cloud")
	c.Assert(loggies[1], qt.Equals, "Fetching Juju agent binaries")

	// Query with two logs, offset 1
	loggies, err = s.Database.QueryBootstrapLog(ctx, jobId, 1)
	c.Assert(err, qt.IsNil)
	c.Assert(loggies[0], qt.Equals, "Fetching Juju agent binaries")

	// Query with two logs, offset 2 (equal to the amount of logs)
	_, err = s.Database.QueryBootstrapLog(ctx, jobId, 2)
	c.Assert(err, qt.ErrorMatches, ".*offset cannot be greater than or equal to the amount of logs.*")
}

// This test is a behaviour check, that is, we want to see that our queued
// locking for the bootstrap_logs table does indeed wait and prevent writes whilst
// it is locked.
func (s *dbSuite) TestBootstrapLogs_lockBootstrapLogs(c *qt.C) {
	ctx := c.Context()

	err := s.Database.Migrate(ctx)
	c.Assert(err, qt.IsNil)

	jobId, err := s.Database.AddJob(ctx, "test-job")
	c.Assert(err, qt.IsNil)

	finishTransaction := make(chan bool)
	lockAcquired := make(chan bool)

	go func() {
		// Simulate a "AddBootstrapLog" call utilising the lockBootstrapLogs func.
		// This enables us to see that locking the table does indeed prevent
		// other AddBootstrapLog calls.
		err := s.Database.Transaction(func(d *db.Database) error {
			err := db.LockBootstrapLogs(d)
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

	// Normal table locks do not support NOWAIT, so this is queue of INSERTS.
	// Meaning, this AddBootstrapLog call will just wait indefinitely until
	// the transaction above finishes.
	//
	// As such we're gonna track the time is above 100ms (best effort test).
	sleepTime := time.Millisecond * 100
	before := time.Now()
	go func() {
		time.Sleep(sleepTime)
		close(finishTransaction)
	}()
	err = s.Database.AddBootstrapLog(ctx, jobId, "Creating Juju controller \"diglett\" on the-most-amazing-cloud")
	c.Assert(err, qt.IsNil)
	after := time.Since(before)

	// We simply check that it taken more than 100ms, as we slept at least 100 and AddBootstrapLog should have
	// taken a few ms too.
	c.Assert(after > sleepTime, qt.IsTrue)
}
