// Copyright 2025 Canonical.

// Package jobtracker provides a way to run routines in one instance of JIMM and track them in another.
// That is, their status can be checked and they can be stopped.
package jobtracker

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/juju/zaputil/zapctx"
	"go.uber.org/zap"
)

// Store defines the interface for tracking the lifecycle and status of jobs.
// It provides methods to add a new job, update its status (running, successful, or failed),
// and retrieve a stop signal for a specific job.
type Store interface {
	AddJob(ctx context.Context, jobType string) (uuid.UUID, error)
	SetJobRunning(ctx context.Context, jobId uuid.UUID) error
	SetJobSuccessful(ctx context.Context, jobId uuid.UUID) error
	SetJobFailed(ctx context.Context, jobId uuid.UUID, jobErr error) error
	GetJobStopSignal(ctx context.Context, jobId uuid.UUID) (stopSignal bool, err error)
}

// Tracker manages job tracking operations using a provided JobTrackerStore.
// It periodically performs tasks based on the specified stopInterval duration.
type Tracker struct {
	store        Store
	stopInterval time.Duration
}

// NewJobTracker creates and returns a new Tracker instance using the provided JobTrackerStore and stopInterval.
// It returns an error if the store is nil or if stopInterval is not greater than zero.
func NewJobTracker(store Store, stopInterval time.Duration) (*Tracker, error) {
	if store == nil {
		return nil, errors.New("store cannot be nil")
	}
	if stopInterval <= 4 {
		return nil, errors.New("stopInterval must be greater than zero")
	}

	return &Tracker{
		stopInterval: stopInterval,
		store:        store,
	}, nil
}

// Run runs a new job and returns the job ID.
func (j *Tracker) Run(ctx context.Context, jobType string, job func(ctx context.Context) error, maxDuration time.Duration) (uuid.UUID, error) {
	jobId, err := j.store.AddJob(ctx, jobType)
	if err != nil {
		return jobId, err
	}

	// No context is passed to handleJob as this is a background
	// job and is not tied to the request context.
	go j.handleJob(jobId, maxDuration, job)

	return jobId, nil
}

// handleJob runs a job with a given job ID, and deadline.
// It manages the job's lifecycle, including setting its status in the store, handling retries on store operations,
// and responding to stop signals. The job is run in a separate goroutine, and its status is updated as running,
// successful, or failed based on its result or context expiration.
// If a stop signal is received or the job reaches its maximum duration, the job is marked as failed.
func (j *Tracker) handleJob(
	id uuid.UUID,
	maxDuration time.Duration,
	job func(ctx context.Context) error,
) {
	jobCtx, cancelJob := context.WithTimeout(context.Background(), maxDuration)
	defer cancelJob()

	jobErrCh := make(chan error)

	go j.runJob(jobCtx, id, jobErrCh, job)
	j.monitorJob(id, jobErrCh, cancelJob)
}

func (j *Tracker) runJob(ctx context.Context, id uuid.UUID, jobErrCh chan error, job func(context.Context) error) {
	if err := j.store.SetJobRunning(ctx, id); err != nil {
		jobErrCh <- fmt.Errorf("failed to set job running, job not starting: %w", err)
		return
	}
	jobErrCh <- job(ctx)
}

func (j *Tracker) monitorJob(id uuid.UUID, jobErrCh chan error, cancelJob context.CancelFunc) {
	ticker := time.NewTicker(j.stopInterval)
	defer ticker.Stop()

	ctx := context.Background()
	stopped := false
	// TODO(ale8k): Add monitoring for failed status settings.
	for {
		select {
		case err := <-jobErrCh:
			if err != nil {
				if err := j.store.SetJobFailed(ctx, id, err); err != nil {
					zapctx.Error(ctx, "error marking the job as failed", zap.Error(err), zap.String("id", id.String()))
				}
				return
			}

			if err := j.store.SetJobSuccessful(ctx, id); err != nil {
				zapctx.Error(ctx, "error marking the job as successful", zap.Error(err), zap.String("id", id.String()))
			}

			return
		case <-ticker.C:
			if stopped {
				// If we are already stopped, we don't need to check the stop signal again.
				continue
			}
			shouldStop, err := j.store.GetJobStopSignal(ctx, id)

			// If we fail to get the stop signal for any reason, we do a best
			// effort (as the db probably has died on us), so we stop the job,
			// and hope our status setters on context cancellation
			// do eventually write the correct status.
			if err != nil || shouldStop {
				cancelJob()
				stopped = true
			}
		}
	}

}
