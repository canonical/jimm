// Copyright 2025 Canonical.
package jobtracker_test

import (
	"context"
	"errors"
	"testing"
	"time"

	qt "github.com/frankban/quicktest"
	"github.com/frankban/quicktest/qtsuite"

	"github.com/canonical/jimm/v3/internal/db"
	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/jobtracker"
	"github.com/canonical/jimm/v3/internal/testutils/jimmtest"
)

type jobTrackerSuite struct {
	db      *db.Database
	tracker *jobtracker.Tracker
}

func (s *jobTrackerSuite) Init(c *qt.C) {
	db := &db.Database{
		DB: jimmtest.PostgresDB(c, time.Now),
	}
	err := db.Migrate(context.Background())
	c.Assert(err, qt.IsNil)

	s.db = db
	tracker, err := jobtracker.NewJobTracker(db, time.Second*5)
	c.Assert(err, qt.IsNil)
	s.tracker = tracker
}

func (s *jobTrackerSuite) TestRun_JobError(c *qt.C) {
	testCtx := c.Context()

	aFastDyingJob := func(ctx context.Context) error {
		return errors.New("I died really fast")
	}

	id, err := s.tracker.Run(testCtx, "test-job-type", aFastDyingJob, time.Second*1000)
	c.Assert(err, qt.IsNil)

	var status dbmodel.JobStatus
	var pollerr error
	for i := 0; i < 10; i++ {
		status, pollerr = s.db.GetJobStatus(testCtx, id)
		c.Assert(pollerr, qt.IsNil)
		if status == dbmodel.StatusFailed {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	j := dbmodel.JobTrackerEntry{}
	var jobErr string
	c.Assert(s.db.DB.First(&j).Where("job_id = ?", id).Select("error").Scan(&jobErr).Error, qt.IsNil)
	c.Assert(jobErr, qt.Equals, "I died really fast")
}

func (s *jobTrackerSuite) TestRun_DeadlineExceeded(c *qt.C) {
	testCtx := c.Context()

	aDeadendJob := func(ctx context.Context) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(10 * time.Second):
			return nil
		}
	}

	id, err := s.tracker.Run(testCtx, "test-job-type", aDeadendJob, time.Millisecond*100)
	c.Assert(err, qt.IsNil)

	var status dbmodel.JobStatus
	var pollerr error
	for i := 0; i < 10; i++ {
		status, pollerr = s.db.GetJobStatus(testCtx, id)
		c.Assert(pollerr, qt.IsNil)
		if status == dbmodel.StatusFailed {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	c.Assert(status, qt.Equals, dbmodel.StatusFailed)

	j := dbmodel.JobTrackerEntry{}
	var jobErr string
	c.Assert(s.db.DB.First(&j).Where("job_id = ?", id).Select("error").Scan(&jobErr).Error, qt.IsNil)
	c.Assert(jobErr, qt.Equals, "context deadline exceeded")
}

func (s *jobTrackerSuite) TestRun_CancelledJob(c *qt.C) {
	testCtx := c.Context()

	// Create a new tracker, specific to this test.
	tracker, err := jobtracker.NewJobTracker(s.db, time.Millisecond*50)
	c.Assert(err, qt.IsNil)

	aStoppedJob := func(ctx context.Context) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(10 * time.Second):
			return errors.New("timedout")
		}
	}

	id, err := tracker.Run(testCtx, "test-job-type", aStoppedJob, time.Second*1000)
	c.Assert(err, qt.IsNil)

	c.Assert(s.db.StopJob(testCtx, id), qt.IsNil)

	var status dbmodel.JobStatus
	var pollerr error
	for i := 0; i < 10; i++ {
		status, pollerr = s.db.GetJobStatus(testCtx, id)
		c.Assert(pollerr, qt.IsNil)
		if status == dbmodel.StatusFailed {
			break
		}
		time.Sleep(10 * time.Second)
	}

	c.Assert(status, qt.Equals, dbmodel.StatusFailed)

	j := dbmodel.JobTrackerEntry{}
	var jobErr string
	c.Assert(s.db.DB.First(&j).Where("job_id = ?", id).Select("error").Scan(&jobErr).Error, qt.IsNil)
	c.Assert(jobErr, qt.Equals, "context canceled")
}

func (s *jobTrackerSuite) TestRun_JobSetRunning(c *qt.C) {
	testCtx := c.Context()

	jobRunning := make(chan bool)
	aRunnignJob := func(ctx context.Context) error {
		jobRunning <- true
		for range ctx.Done() {
		}
		return nil
	}

	id, err := s.tracker.Run(testCtx, "test-job-type", aRunnignJob, time.Second*1000)
	c.Assert(err, qt.IsNil)

	<-jobRunning
	status, err := s.db.GetJobStatus(testCtx, id)
	c.Assert(err, qt.IsNil)
	c.Assert(status, qt.Equals, dbmodel.StatusRunning)
}

func (s *jobTrackerSuite) TestRun_JobSetSuccessful(c *qt.C) {
	testCtx := c.Context()

	aSuccessfulJob := func(ctx context.Context) error {
		return nil
	}

	id, err := s.tracker.Run(testCtx, "test-job-type", aSuccessfulJob, time.Second*1000)
	c.Assert(err, qt.IsNil)

	var status dbmodel.JobStatus
	var pollerr error
	for i := 0; i < 10; i++ {
		status, pollerr = s.db.GetJobStatus(testCtx, id)
		c.Assert(pollerr, qt.IsNil)
		if status == dbmodel.StatusSuccessful {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	c.Assert(status, qt.Equals, dbmodel.StatusSuccessful)
}

func TestJobTrackerSuite(t *testing.T) {
	qtsuite.Run(qt.New(t), &jobTrackerSuite{})
}
