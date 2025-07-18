// Copyright 2025 Canonical.

package bootstrap_test

import (
	"context"
	"fmt"
	"math"
	"testing"
	"time"

	qt "github.com/frankban/quicktest"
	"github.com/frankban/quicktest/qtsuite"
	"github.com/google/uuid"

	"github.com/canonical/jimm/v3/internal/db"
	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/jimm/bootstrap"
	"github.com/canonical/jimm/v3/internal/jobtracker"
	"github.com/canonical/jimm/v3/internal/openfga"
	"github.com/canonical/jimm/v3/internal/testutils/jimmtest"
	"github.com/canonical/jimm/v3/pkg/api/params"
)

type bootstrapManagerSuite struct {
	manager    *bootstrap.BootstrapManager
	jobTracker *jobtracker.Tracker
	adminUser  *openfga.User
	db         *db.Database
	ofgaClient *openfga.OFGAClient
}

func (s *bootstrapManagerSuite) Init(c *qt.C) {
	db := &db.Database{
		DB: jimmtest.PostgresDB(c, time.Now),
	}
	err := db.Migrate(context.Background())
	c.Assert(err, qt.IsNil)

	s.db = db

	ofgaClient, _, _, err := jimmtest.SetupTestOFGAClient(c.Name())
	c.Assert(err, qt.IsNil)

	s.ofgaClient = ofgaClient

	jobtracker, err := jobtracker.NewJobTracker(db, 1*time.Minute)
	s.jobTracker = jobtracker
	c.Assert(err, qt.IsNil)
	s.manager, err = bootstrap.NewBootstrapManager(db, ofgaClient, jobtracker)
	c.Assert(err, qt.IsNil)
}

func (s *bootstrapManagerSuite) TestGetBootstrapStatusAndLogs(c *qt.C) {
	ctx := c.Context()
	read := make(chan struct{})
	defer close(read)
	write := make(chan struct{})
	defer close(write)

	numLogs := 101
	batchSize := 10

	jobId, err := s.jobTracker.Run(ctx,
		"bootstrap-job",
		func(ctx context.Context) error {
			jobId, ok := jobtracker.JobIdFromContext(ctx)
			c.Check(ok, qt.IsTrue)

			for i := range numLogs {
				if i%batchSize == 0 && i > 0 {
					write <- struct{}{} // Signal that a batch of logs has been written.
					<-read              // Wait for the read before writing the next batch.
				}
				err := s.db.AddBootstrapLog(ctx, jobId, "bootstrap logs "+fmt.Sprint(rune(i)))
				c.Check(err, qt.IsNil)
			}
			// We need to signal that we've written the last batch of logs.
			write <- struct{}{}
			<-read
			return nil
		},
		1*time.Minute,
	)
	c.Assert(err, qt.IsNil)
	watermark := 0
	for batch := 0; batch < numLogs/batchSize+1; batch++ {
		<-write // Wait for the batch of logs to be written.

		response, err := s.manager.GetBootstrapStatusAndLogs(ctx, s.adminUser, jobId, watermark)
		c.Assert(err, qt.IsNil)
		logs := []string{}
		for j := 0; j < int(math.Min(float64(batchSize), float64(numLogs-batch*batchSize))); j++ {
			logs = append(logs, "bootstrap logs "+fmt.Sprint(rune(batch*batchSize+j)))
		}
		c.Check(response.Logs, qt.DeepEquals, logs)
		c.Assert(response.Status, qt.Equals, string(dbmodel.StatusRunning))
		watermark = response.Watermark

		read <- struct{}{} // Signal it has been read.
	}

	// check last batch is empty.
	response, err := s.manager.GetBootstrapStatusAndLogs(ctx, s.adminUser, jobId, watermark)
	c.Assert(response.Status == string(dbmodel.StatusSuccessful) || response.Status == string(dbmodel.StatusRunning), qt.IsTrue)
	c.Assert(err, qt.IsNil)
	c.Assert(response.Logs, qt.HasLen, 0)
}

func (s *bootstrapManagerSuite) TestGetBootstrapStatusAndLogs_JobFailed(c *qt.C) {
	ctx := c.Context()
	jobId, err := s.jobTracker.Run(ctx,
		"bootstrap-job",
		func(ctx context.Context) error {
			return fmt.Errorf("I died really fast")
		},
		1*time.Minute,
	)
	c.Assert(err, qt.IsNil)
	var response params.BootstrapStatusResponse
	for range 10 {
		response, err = s.manager.GetBootstrapStatusAndLogs(ctx, s.adminUser, jobId, 0)
		c.Assert(err, qt.IsNil)
		if response.Status == string(dbmodel.StatusFailed) {
			break
		}
		time.Sleep(100 * time.Millisecond) // Wait for the job to be marked as failed.
	}
	c.Assert(response.Status, qt.Equals, string(dbmodel.StatusFailed))
	c.Assert(response.Error, qt.Equals, "I died really fast")
}

func (s *bootstrapManagerSuite) TestGetBootstrapStatusAndLogs_JobNotFound(c *qt.C) {
	ctx := c.Context()
	jobId := uuid.New()
	_, err := s.manager.GetBootstrapStatusAndLogs(ctx, s.adminUser, jobId, 0)
	c.Assert(err, qt.ErrorMatches, "failed to get job status")
}

func TestBootstrapManager(t *testing.T) {
	qtsuite.Run(qt.New(t), &bootstrapManagerSuite{})
}
