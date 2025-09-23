// Copyright 2025 Canonical.

package dbmodel_test

import (
	"testing"

	qt "github.com/frankban/quicktest"
	"github.com/frankban/quicktest/qtsuite"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/canonical/jimm/v3/internal/dbmodel"
)

type jobLogsSuite struct {
	Database *gorm.DB
}

func (j *jobLogsSuite) Init(c *qt.C) {
	j.Database = gormDB(c)
}

func (j *jobLogsSuite) TestJobTracker_NewJobLog(c *qt.C) {
	id := uuid.New()

	_, err := dbmodel.NewJobLog(uuid.UUID{}, -1, "test log line")
	c.Assert(err, qt.ErrorMatches, ".*job id cannot be nil.*")

	_, err = dbmodel.NewJobLog(id, -1, "test log line")
	c.Assert(err, qt.ErrorMatches, ".*line number must be non-negative.*")

	_, err = dbmodel.NewJobLog(id, 0, "")
	c.Assert(err, qt.ErrorMatches, ".*log line cannot be empty.*")
}

func (j *jobLogsSuite) TestJobLog(c *qt.C) {
	db := j.Database

	// Test a bad log entry
	badEntry := &dbmodel.JobLog{}
	c.Assert(db.First(badEntry).Error, qt.IsNotNil)

	// Test with invalid FK
	badJobId := uuid.New()
	entry, err := dbmodel.NewJobLog(badJobId, 0, "hi")
	c.Assert(err, qt.IsNil)
	c.Assert(db.Create(entry).Error, qt.ErrorMatches, ".*violates foreign key constraint.*")

	// Test a valid log entry

	// Create a job
	job, err := dbmodel.NewJobTrackerEntry("test-job")
	c.Assert(err, qt.IsNil)
	c.Assert(db.Create(job).Error, qt.IsNil)

	// Create a log entry for the job
	entry, err = dbmodel.NewJobLog(job.JobID, 0, "Creating Juju controller \"aws-controller\" on aws/us-east-1")
	c.Assert(err, qt.IsNil)
	c.Assert(db.Create(entry).Error, qt.IsNil)

	// Quickly grab the log in a fresh entry
	log := &dbmodel.JobLog{}
	c.Assert(db.First(log, "job_id = ? AND line_number = ?", job.JobID, 0).Error, qt.IsNil)
	c.Assert(log.JobID, qt.Equals, job.JobID)
	c.Assert(log.LineNumber, qt.Equals, 0)
	c.Assert(log.LogLine, qt.Equals, "Creating Juju controller \"aws-controller\" on aws/us-east-1")
}

func TestJobLogsSuite(t *testing.T) {
	qtsuite.Run(qt.New(t), &jobLogsSuite{})
}
