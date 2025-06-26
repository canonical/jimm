// Copyright 2025 Canonical.

package db

import (
	"context"

	"github.com/google/uuid"

	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/servermon"
)

func (d *Database) AddBootstrapLog(ctx context.Context, jobId uuid.UUID, logLine string) (err error) {
	const op = errors.Op("db.AddBootstrapLog")

	if err := d.ready(); err != nil {
		return errors.E(op, err)
	}

	return d.Transaction(func(d *Database) error {
		// Lock entire table at start as we're only allowing one bootstrap at a time.
		if err := d.DB.Exec("LOCK TABLE bootstrap_logs IN EXCLUSIVE MODE").Error; err != nil {
			return errors.E(op, "failed to lock table", err)
		}

		// Get the current line number for this bootstrap job.
		var currentLineNumber int
		err = d.DB.WithContext(ctx).
			Model(&dbmodel.BootstrapLog{}).
			Where("job_id = ?", jobId).
			Select("COALESCE(MAX(line_number), 0)").
			Scan(&currentLineNumber).Error
		if err != nil {
			return errors.E(op, "failed to get current line number", err)
		}

		nextLineNumber := currentLineNumber + 1

		log, err := dbmodel.NewBootstrapLog(jobId, nextLineNumber, logLine)
		if err != nil {
			return errors.E(op, "failed to construct bootstrap log", err)
		}

		if err := d.DB.WithContext(ctx).Create(log).Error; err != nil {
			return errors.E(op, dbError(err))
		}
		return nil
	})
}

// QueryBootstrapLog queries for bootstrap logs based on the jobId and offset.
// It returns an error if the job doesn't exist, there aren't any logs or the offset
// is greater than the amount of logs,
func (d *Database) QueryBootstrapLog(ctx context.Context, jobId uuid.UUID, offset int) (loggies []string, err error) {
	const op = errors.Op("db.QueryBootstrapLog")

	if err := d.ready(); err != nil {
		return loggies, errors.E(op, err)
	}

	durationObserver := servermon.DurationObserver(servermon.DBQueryDurationHistogram, string(op))
	defer durationObserver()
	defer servermon.ErrorCounter(servermon.DBQueryErrorCount, &err, string(op))

	// Make sure job exists, if it doesn't, there's no point running the query
	if err := d.DB.WithContext(ctx).First(&dbmodel.JobTrackerEntry{JobID: jobId}, "job_id = ?", jobId).Error; err != nil {
		return loggies, errors.E(op, "job not found", dbError(err))
	}

	query := d.DB.WithContext(ctx).
		Model(&dbmodel.BootstrapLog{}).
		Where("job_id = ?", jobId).
		Order("line_number ASC")

	var count int64
	if err := query.Count(&count).Error; err != nil {
		return loggies, errors.E(op, dbError(err))
	}

	if count == 0 {
		return loggies, errors.E(op, errors.CodeNotFound)
	}

	// Validate the offset isn't greater than the amount of actual logs
	if int64(offset) >= count {
		return loggies, errors.E(op, "offset cannot be greater than or equal to the amount of logs")
	}

	var logs []dbmodel.BootstrapLog
	result := query.Offset(offset).Find(&logs)
	if result.Error != nil {
		return loggies, errors.E(op, dbError(result.Error))
	}

	for _, l := range logs {
		loggies = append(loggies, l.LogLine)
	}

	return loggies, nil
}
