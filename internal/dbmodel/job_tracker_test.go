// Copyright 2025 Canonical.

package dbmodel_test

import (
	"errors"
	"testing"

	qt "github.com/frankban/quicktest"
	"github.com/frankban/quicktest/qtsuite"
	"gorm.io/gorm"

	"github.com/canonical/jimm/v3/internal/dbmodel"
)

type jobTrackerSuite struct {
	Database *gorm.DB
}

func (j *jobTrackerSuite) Init(c *qt.C) {
	j.Database = gormDB(c)
}

func (j *jobTrackerSuite) TestJobTracker(c *qt.C) {
	db := j.Database

	badEntry := &dbmodel.JobTrackerEntry{}
	c.Assert(db.First(badEntry).Error, qt.IsNotNil)

	entry, err := dbmodel.NewJobTrackerEntry("test-job")
	c.Assert(err, qt.IsNil)
	c.Assert(db.Create(entry).Error, qt.IsNil)

	c.Assert(entry.Error, qt.Equals, "")

	// Now we set the status, update it, and check the error in a fresh instance.
	c.Assert(entry.SetFailed(errors.New("test error")), qt.IsNil)
	c.Assert(db.Save(entry).Error, qt.IsNil)

	entry2 := dbmodel.JobTrackerEntry{JobID: entry.JobID}
	c.Assert(db.First(&entry2).Error, qt.IsNil)
	c.Assert(entry2.Status, qt.Equals, dbmodel.StatusFailed)
	c.Assert(entry2.Error, qt.Equals, "test error")
}

func (j *jobTrackerSuite) TestJobTracker_CannotSetArbritaryStatus(c *qt.C) {
	db := j.Database

	badEntry := &dbmodel.JobTrackerEntry{}
	c.Assert(db.First(badEntry).Error, qt.IsNotNil)

	entry, err := dbmodel.NewJobTrackerEntry("test-job")
	c.Assert(err, qt.IsNil)
	c.Assert(db.Create(entry).Error, qt.IsNil)

	entry.Status = "not a real status"
	c.Assert(db.Save(entry).Error, qt.ErrorMatches, ".*invalid input value for enum job_tracker_status.*")
}

func (j *jobTrackerSuite) TestJobTracker_StatusesSetCorrectly(c *qt.C) {
	entry := dbmodel.JobTrackerEntry{}

	err := entry.SetFailed(nil)
	c.Assert(err, qt.ErrorMatches, ".*error cannot be nil.*")

	err = entry.SetFailed(errors.New("test error"))
	c.Assert(err, qt.IsNil)
	c.Assert(entry.Status, qt.Equals, dbmodel.StatusFailed)
	c.Assert(entry.Error, qt.Equals, "test error")

	entry.Error = "some error"
	entry.SetRunning()
	c.Assert(entry.Status, qt.Equals, dbmodel.StatusRunning)
	c.Assert(entry.Error, qt.Equals, "")

	entry.Error = "some error"
	entry.SetSuccessful()
	c.Assert(entry.Status, qt.Equals, dbmodel.StatusSuccessful)
	c.Assert(entry.Error, qt.Equals, "")

	entry.Status = dbmodel.StatusPending
	c.Assert(entry.GetStatus(), qt.Equals, dbmodel.StatusPending)
}

func TestJobTrackerSuite(t *testing.T) {
	qtsuite.Run(qt.New(t), &jobTrackerSuite{})
}
