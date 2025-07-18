// Copyright 2025 Canonical.
package jobtracker_test

import (
	"context"
	"errors"
	"testing"
	"time"

	qt "github.com/frankban/quicktest"
	"github.com/frankban/quicktest/qtsuite"
	"github.com/google/uuid"

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

func (s *jobTrackerSuite) pollJob(ctx context.Context, id uuid.UUID, c *qt.C, expectedStatus dbmodel.JobStatus) {
	var status dbmodel.JobStatus
	var pollerr error
	for i := 0; i < 10; i++ {
		status, pollerr = s.db.GetJobStatus(ctx, id)
		c.Assert(pollerr, qt.IsNil)
		if status == expectedStatus {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	c.Assert(status, qt.Equals, expectedStatus)
}

func (s *jobTrackerSuite) TestRun_JobError(c *qt.C) {
	testCtx := c.Context()

	aFastDyingJob := func(ctx context.Context) error {
		return errors.New("I died really fast")
	}

	id, err := s.tracker.Run(testCtx, "test-job-type", aFastDyingJob, time.Second*1000)
	c.Assert(err, qt.IsNil)

	s.pollJob(testCtx, id, c, dbmodel.StatusFailed)

	j := dbmodel.JobTrackerEntry{}
	var jobErr string
	c.Assert(s.db.DB.First(&j).Where("job_id = ?", id).Select("error").Scan(&jobErr).Error, qt.IsNil)
	c.Assert(jobErr, qt.Equals, "I died really fast")
}

func (s *jobTrackerSuite) TestRun_JobSetRunning(c *qt.C) {
	testCtx := c.Context()

	jobRunning := make(chan bool)
	jobCtx, cancelJobCtx := context.WithCancel(testCtx)
	aRunnignJob := func(_ context.Context) error {
		jobRunning <- true
		for range jobCtx.Done() {
		}
		return nil
	}

	id, err := s.tracker.Run(testCtx, "test-job-type", aRunnignJob, time.Second*1000)
	c.Assert(err, qt.IsNil)

	<-jobRunning
	status, err := s.db.GetJobStatus(testCtx, id)
	c.Assert(err, qt.IsNil)
	c.Assert(status, qt.Equals, dbmodel.StatusRunning)

	// Check the job is marked successful on completion.
	cancelJobCtx()
	s.pollJob(testCtx, id, c, dbmodel.StatusSuccessful)
}

func (s *jobTrackerSuite) TestRun_JobSetSuccessful(c *qt.C) {
	testCtx := c.Context()

	aSuccessfulJob := func(ctx context.Context) error {
		return nil
	}

	id, err := s.tracker.Run(testCtx, "test-job-type", aSuccessfulJob, time.Second*1000)
	c.Assert(err, qt.IsNil)

	s.pollJob(testCtx, id, c, dbmodel.StatusSuccessful)
}

func (s *jobTrackerSuite) TestRun_JobIdSetInContext(c *qt.C) {
	testCtx := c.Context()

	aJobWithIdInContext := func(ctx context.Context) error {
		jobId, ok := jobtracker.JobIdFromContext(ctx)
		c.Check(jobId, qt.Not(qt.Equals), uuid.Nil)
		c.Check(ok, qt.IsTrue)
		return nil
	}

	id, err := s.tracker.Run(testCtx, "test-job-type", aJobWithIdInContext, time.Second*1000)
	c.Assert(err, qt.IsNil)

	s.pollJob(testCtx, id, c, dbmodel.StatusSuccessful)
}

func TestJobTrackerSuite(t *testing.T) {
	qtsuite.Run(qt.New(t), &jobTrackerSuite{})
}
