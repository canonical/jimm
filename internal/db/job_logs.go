// Copyright 2025 Canonical.

package db

import (
	"context"
	"fmt"

	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/servermon"
)

var (
	// Blocks most concurrent writes and schema changes but allows reads.
	jobLoglockQuery = "LOCK TABLE job_logs IN EXCLUSIVE MODE"
)

// AddJobLog adds a job log entry to the store.
func (d *Database) AddJobLog(ctx context.Context, jobId int64, logLine string) (err error) {
	const op = "db.AddJobLog"

	if err := d.ready(); err != nil {
		return err
	}

	durationObserver := servermon.DurationObserver(servermon.DBQueryDurationHistogram, op)
	defer durationObserver()
	defer servermon.ErrorCounter(servermon.DBQueryErrorCount, &err, op)

	return d.Transaction(func(d *Database) error {
		// Blocks all other operations, including reads, writes, and other locks.
		if err := d.DB.Exec(jobLoglockQuery).Error; err != nil {
			return errors.E("failed to lock job_logs table", err)
		}

		// Get the current line number for this job.
		var currentLineNumber int
		err = d.DB.WithContext(ctx).
			Model(&dbmodel.JobLog{}).
			Where("job_id = ?", jobId).
			Select("COALESCE(MAX(line_number), 0)").
			Scan(&currentLineNumber).Error
		if err != nil {
			return errors.E("failed to get current line number", err)
		}

		nextLineNumber := currentLineNumber + 1

		log, err := dbmodel.NewJobLog(jobId, nextLineNumber, logLine)
		if err != nil {
			return fmt.Errorf("failed to construct job log: %v", err)
		}

		if err := d.DB.WithContext(ctx).Create(log).Error; err != nil {
			return errors.E(dbError(err))
		}
		return nil
	})
}

// QueryJobLog queries for job logs based on the jobId and offset.
//
// It returns the next offset value to use, and this offset value may be the same
// as the one initially presented / previously returned. This means no new logs have
// come in, but they may later, and the client should query again for logs after some time.
func (d *Database) QueryJobLog(ctx context.Context, jobId int64, offset int) (loggies []string, nextOffsetValue int, err error) {
	const op = "db.QueryJobLog"

	if err := d.ready(); err != nil {
		return loggies, nextOffsetValue, err
	}

	durationObserver := servermon.DurationObserver(servermon.DBQueryDurationHistogram, op)
	defer durationObserver()
	defer servermon.ErrorCounter(servermon.DBQueryErrorCount, &err, op)

	var logs []dbmodel.JobLog
	err = d.Transaction(func(d *Database) error {
		query := d.DB.WithContext(ctx).
			Model(&dbmodel.JobLog{}).
			Where("job_id = ?", jobId)

		var count int64
		if err := query.Count(&count).Error; err != nil {
			return errors.E(dbError(err))
		}

		if count == 0 {
			return nil
		}

		result := query.Offset(offset).Order("line_number ASC").Find(&logs)
		if result.Error != nil {
			return errors.E(dbError(result.Error))
		}

		// Get the next line number
		var currentLineNumber int
		err = d.DB.WithContext(ctx).
			Model(&dbmodel.JobLog{}).
			Where("job_id = ?", jobId).
			Select("COALESCE(MAX(line_number), 0)").
			Scan(&currentLineNumber).Error
		if err != nil {
			return errors.E("failed to get current line number", err)
		}

		nextOffsetValue = currentLineNumber
		return nil
	})
	if err != nil {
		return loggies, nextOffsetValue, err
	}

	for _, l := range logs {
		loggies = append(loggies, l.LogLine)
	}

	return loggies, nextOffsetValue, nil
}
