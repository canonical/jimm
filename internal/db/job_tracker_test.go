// Copyright 2025 Canonical.

package db_test

import (
	"errors"

	qt "github.com/frankban/quicktest"
	"github.com/google/uuid"

	"github.com/canonical/jimm/v3/internal/dbmodel"
)

func (s *dbSuite) TestJobTracker_CreateJob(c *qt.C) {
	ctx := c.Context()
	err := s.Database.Migrate(ctx)
	c.Assert(err, qt.IsNil)

	jobId, err := s.Database.AddJob(ctx, "test-job-type")
	c.Assert(err, qt.IsNil)

	entry := &dbmodel.JobTrackerEntry{}
	err = s.Database.DB.First(entry, "job_id = ?", jobId).Error
	c.Assert(err, qt.IsNil)

	c.Assert(entry.JobID, qt.Equals, jobId)
	c.Assert(entry.JobType, qt.Equals, "test-job-type")
	c.Assert(entry.Status, qt.Equals, dbmodel.StatusPending)
	c.Assert(entry.StopSignal, qt.IsFalse)
	c.Assert(entry.Error, qt.Equals, "")
}

func (s *dbSuite) TestJobTracker_GetJobStopSignal(c *qt.C) {
	ctx := c.Context()
	_, err := s.Database.GetJobStopSignal(ctx, uuid.New())
	c.Assert(err, qt.ErrorMatches, "upgrade in progress")

	err = s.Database.Migrate(ctx)
	c.Assert(err, qt.IsNil)

	_, err = s.Database.GetJobStopSignal(ctx, uuid.New())
	c.Assert(err, qt.ErrorMatches, ".*not found.*")

	jobId, err := s.Database.AddJob(ctx, "test-job-type")
	c.Assert(err, qt.IsNil)

	// By default, stop signal should be false
	stopSignal, err := s.Database.GetJobStopSignal(ctx, jobId)
	c.Assert(err, qt.IsNil)
	c.Assert(stopSignal, qt.IsFalse)

	// Set stop signal
	err = s.Database.StopJob(ctx, jobId)
	c.Assert(err, qt.IsNil)

	// Now stop signal should be true
	stopSignal, err = s.Database.GetJobStopSignal(ctx, jobId)
	c.Assert(err, qt.IsNil)
	c.Assert(stopSignal, qt.IsTrue)
}

func (s *dbSuite) TestJobTracker_StopJob(c *qt.C) {
	ctx := c.Context()
	err := s.Database.Migrate(ctx)
	c.Assert(err, qt.IsNil)

	err = s.Database.StopJob(ctx, uuid.New())
	c.Assert(err, qt.ErrorMatches, ".*not found.*")

	jobId, err := s.Database.AddJob(ctx, "test-job-type")
	c.Assert(err, qt.IsNil)

	err = s.Database.StopJob(ctx, jobId)
	c.Assert(err, qt.IsNil)

	entry := &dbmodel.JobTrackerEntry{}
	err = s.Database.DB.First(entry, "job_id = ?", jobId).Error
	c.Assert(err, qt.IsNil)

	c.Assert(entry.StopSignal, qt.IsTrue)
}

func (s *dbSuite) TestJobTracker_StatusSetters(c *qt.C) {
	ctx := c.Context()
	err := s.Database.Migrate(ctx)
	c.Assert(err, qt.IsNil)

	err = s.Database.SetJobRunning(ctx, uuid.New())
	c.Assert(err, qt.ErrorMatches, ".*not found.*")

	jobId, err := s.Database.AddJob(ctx, "test-job-type")
	c.Assert(err, qt.IsNil)

	// Pending
	entry := &dbmodel.JobTrackerEntry{}
	err = s.Database.DB.First(entry, "job_id = ?", jobId).Error
	c.Assert(err, qt.IsNil)
	c.Assert(entry.Status, qt.Equals, dbmodel.StatusPending)
	c.Assert(entry.Error, qt.Equals, "")

	// Failed
	err = s.Database.SetJobFailed(ctx, jobId, errors.New("test error"))
	c.Assert(err, qt.IsNil)

	err = s.Database.DB.First(entry, "job_id = ?", jobId).Error
	c.Assert(err, qt.IsNil)
	c.Assert(entry.Status, qt.Equals, dbmodel.StatusFailed)
	c.Assert(entry.Error, qt.Equals, "test error")

	// Running
	err = s.Database.SetJobRunning(ctx, jobId)
	c.Assert(err, qt.IsNil)

	err = s.Database.DB.First(entry, "job_id = ?", jobId).Error
	c.Assert(err, qt.IsNil)
	c.Assert(entry.Status, qt.Equals, dbmodel.StatusRunning)
	c.Assert(entry.Error, qt.Equals, "")

	// Successful
	err = s.Database.SetJobSuccessful(ctx, jobId)
	c.Assert(err, qt.IsNil)

	err = s.Database.DB.First(entry, "job_id = ?", jobId).Error
	c.Assert(err, qt.IsNil)
	c.Assert(entry.Status, qt.Equals, dbmodel.StatusSuccessful)
	c.Assert(entry.Error, qt.Equals, "")
}

func (s *dbSuite) TestJobTracker_GetStatus(c *qt.C) {
	ctx := c.Context()
	err := s.Database.Migrate(ctx)
	c.Assert(err, qt.IsNil)

	_, err = s.Database.GetJobStatus(ctx, uuid.New())
	c.Assert(err, qt.ErrorMatches, ".*not found.*")

	jobId, err := s.Database.AddJob(ctx, "test-job-type")
	c.Assert(err, qt.IsNil)

	status, err := s.Database.GetJobStatus(ctx, jobId)
	c.Assert(err, qt.IsNil)
	c.Assert(status, qt.Equals, dbmodel.StatusPending)
}
