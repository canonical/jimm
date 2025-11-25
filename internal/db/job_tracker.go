// Copyright 2025 Canonical.

package db

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/juju/zaputil/zapctx"
	"go.uber.org/zap"

	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/servermon"
)

// AddJob adds a new job to the job tracker.
// It returns the job ID and an error if the job already exists or if the creation fails
func (d *Database) AddJob(ctx context.Context, jobType string) (jobId uuid.UUID, err error) {
	const op = "db.AddJob"
	if err := d.ready(); err != nil {
		return jobId, errors.E(err)
	}

	durationObserver := servermon.DurationObserver(servermon.DBQueryDurationHistogram, op)
	defer durationObserver()
	defer servermon.ErrorCounter(servermon.DBQueryErrorCount, &err, op)

	db := d.DB.WithContext(ctx)

	entry, err := dbmodel.NewJobTrackerEntry(jobType)
	if err != nil {
		return jobId, errors.E(fmt.Sprintf("failed to create new job tracker entry: %v", err))
	}
	if err := db.Create(entry).Error; err != nil {
		err := dbError(err)
		if errors.ErrorCode(err) == errors.CodeAlreadyExists {
			zapctx.Debug(ctx, "job already exists", zap.String("jobID", entry.JobID.String()))
			return jobId, errors.E(fmt.Sprintf("job %s already exists", entry.JobID), err)
		}
		return jobId, errors.E(err)
	}

	jobId = entry.JobID
	return jobId, nil
}

// GetJob retrieves a job entry by its ID.
func (d *Database) GetJob(ctx context.Context, job *dbmodel.JobTrackerEntry) error {
	const op = "db.GetJob"
	var err error
	if err := d.ready(); err != nil {
		return errors.E(err)
	}

	durationObserver := servermon.DurationObserver(servermon.DBQueryDurationHistogram, op)
	defer durationObserver()
	defer servermon.ErrorCounter(servermon.DBQueryErrorCount, &err, op)

	if job.JobID == uuid.Nil {
		return errors.E(errors.CodeBadRequest, "job ID cannot be empty")
	}
	db := d.DB.WithContext(ctx)
	if err := db.Where("job_id = ?", job.JobID).First(&job).Error; err != nil {
		return dbError(err)
	}

	return nil
}

// GetJobStopSignal retrieves the stop signal for a job by its ID.
// It returns true if the stop signal is set, false otherwise, and an error if the job does not exist or if the query fails.
func (d *Database) GetJobStopSignal(ctx context.Context, jobId uuid.UUID) (stopSignal bool, err error) {
	const op = "db.GetJobStopSignal"
	if err := d.ready(); err != nil {
		return false, errors.E(err)
	}

	durationObserver := servermon.DurationObserver(servermon.DBQueryDurationHistogram, op)
	defer durationObserver()
	defer servermon.ErrorCounter(servermon.DBQueryErrorCount, &err, op)

	db := d.DB.WithContext(ctx)
	var entry dbmodel.JobTrackerEntry
	if err := db.Select("stop_signal").Where("job_id = ?", jobId).First(&entry).Error; err != nil {
		return false, dbError(err)
	}

	return entry.StopSignal, nil
}

// StopJob marks a job as stopped.
// It returns an error if the job does not exist or if the update fails.
func (d *Database) StopJob(ctx context.Context, jobId uuid.UUID) (err error) {
	const op = "db.StopJob"
	if err := d.ready(); err != nil {
		return errors.E(err)
	}

	durationObserver := servermon.DurationObserver(servermon.DBQueryDurationHistogram, op)
	defer durationObserver()
	defer servermon.ErrorCounter(servermon.DBQueryErrorCount, &err, op)

	db := d.DB.WithContext(ctx)

	result := db.Model(&dbmodel.JobTrackerEntry{}).Where("job_id = ?", jobId).Update("stop_signal", true)
	if err := result.Error; err != nil {
		return dbError(err)
	}

	if result.RowsAffected == 0 {
		return errors.E(errors.CodeNotFound, fmt.Sprintf("job %s not found", jobId))
	}

	return nil
}

// SetJobSuccessful sets the job status to successful.
// It returns an error if the job does not exist or if the update fails.
func (d *Database) SetJobRunning(ctx context.Context, jobId uuid.UUID) (err error) {
	entry := dbmodel.JobTrackerEntry{
		JobID: jobId,
	}
	entry.SetRunning()
	return d.updateJob(ctx, entry)
}

// SetJobSuccessful sets the job status to successful.
// It returns an error if the job does not exist or if the update fails.
func (d *Database) SetJobSuccessful(ctx context.Context, jobId uuid.UUID) (err error) {
	entry := dbmodel.JobTrackerEntry{
		JobID: jobId,
	}
	entry.SetSuccessful()
	return d.updateJob(ctx, entry)
}

// SetJobFailed sets the job status to failed and records the error message.
// It returns an error if the job does not exist or if the update fails.
func (d *Database) SetJobFailed(ctx context.Context, jobId uuid.UUID, jobErr error) (err error) {
	entry := dbmodel.JobTrackerEntry{
		JobID: jobId,
	}
	if err := entry.SetFailed(jobErr); err != nil {
		return errors.E(err)
	}

	return d.updateJob(ctx, entry)
}

func (d *Database) updateJob(ctx context.Context, entry dbmodel.JobTrackerEntry) (err error) {
	const op = "db.updateJobStatus"
	if err := d.ready(); err != nil {
		return errors.E(err)
	}

	durationObserver := servermon.DurationObserver(servermon.DBQueryDurationHistogram, op)
	defer durationObserver()
	defer servermon.ErrorCounter(servermon.DBQueryErrorCount, &err, op)

	db := d.DB.WithContext(ctx)
	result := db.Model(&entry).Select("status", "error").Updates(entry)
	if err := result.Error; err != nil {
		return dbError(err)
	}

	if result.RowsAffected == 0 {
		return errors.E(errors.CodeNotFound, fmt.Sprintf("job %s not found", entry.JobID))
	}

	return nil
}

// GetJobStatus retrieves the status of a job by its ID.
// It returns the job status and an error if the job does not exist or if the query fails.
func (d *Database) GetJobStatus(ctx context.Context, jobId uuid.UUID) (status dbmodel.JobStatus, err error) {
	const op = "db.GetJobStatus"
	if err := d.ready(); err != nil {
		return status, errors.E(err)
	}

	durationObserver := servermon.DurationObserver(servermon.DBQueryDurationHistogram, op)
	defer durationObserver()
	defer servermon.ErrorCounter(servermon.DBQueryErrorCount, &err, op)

	db := d.DB.WithContext(ctx)
	entry := &dbmodel.JobTrackerEntry{}
	if err := db.Where("job_id = ?", jobId).First(entry).Error; err != nil {
		return status, dbError(err)
	}

	return entry.GetStatus(), nil
}
