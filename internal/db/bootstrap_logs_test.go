// Copyright 2025 Canonical.

package db_test

import (
	qt "github.com/frankban/quicktest"
	"github.com/google/uuid"

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
	c.Assert(err, qt.ErrorMatches, "no logs")
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
