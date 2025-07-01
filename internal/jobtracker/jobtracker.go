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
	"github.com/juju/clock"
	"github.com/juju/retry"
	"github.com/juju/zaputil/zapctx"
	"go.uber.org/zap"
)

// JobTrackerStore defines the interface for tracking the lifecycle and status of jobs.
// It provides methods to add a new job, update its status (running, successful, or failed),
// and retrieve a stop signal for a specific job.
type JobTrackerStore interface {
	AddJob(ctx context.Context, jobType string) (uuid.UUID, error)
	SetJobRunning(ctx context.Context, jobId uuid.UUID) error
	SetJobSuccessful(ctx context.Context, jobId uuid.UUID) error
	SetJobFailed(ctx context.Context, jobId uuid.UUID, jobErr error) error
	GetJobStopSignal(ctx context.Context, jobId uuid.UUID) (stopSignal bool, err error)
}

// Tracker manages job tracking operations using a provided JobTrackerStore.
// It periodically performs tasks based on the specified stopInterval duration.
type Tracker struct {
	store        JobTrackerStore
	stopInterval time.Duration
}

// NewJobTracker creates and returns a new Tracker instance using the provided JobTrackerStore and stopInterval.
// It returns an error if the store is nil or if stopInterval is not greater than zero.
func NewJobTracker(store JobTrackerStore, stopInterval time.Duration) (*Tracker, error) {
	tracker := &Tracker{}
	if store == nil {
		return tracker, errors.New("store cannot be nil")
	}
	if stopInterval <= 0 {
		return tracker, errors.New("stopInterval must be greater than zero")
	}
	tracker.stopInterval = stopInterval
	tracker.store = store

	return tracker, nil
}

// Run runs a new job and reurns the job ID.
func (j *Tracker) Run(ctx context.Context, jobType string, job func(ctx context.Context) error, deadline time.Duration) (uuid.UUID, error) {
	jobId, err := j.store.AddJob(ctx, jobType)
	if err != nil {
		return jobId, err
	}

	stopPollInterval := time.Second * 5

	go j.runJob(ctx, jobId, stopPollInterval, deadline, job)

	return jobId, nil
}

// runJob runs a job with a given context, job ID, polling interval, and deadline.
// It manages the job's lifecycle, including setting its status in the store, handling retries on store operations,
// and responding to stop signals or context cancellations. The job is run in a separate goroutine, and its status
// is updated as running, successful, or failed based on its result or context expiration.
// If a stop signal is received or the context is canceled or times out, the job is marked as failed.
// Store operations are retried up to 5 times with a 30-second delay between attempts in case of transient errors.
func (j *Tracker) runJob(
	ctx context.Context,
	id uuid.UUID,
	stopPollInterval time.Duration,
	deadline time.Duration,
	job func(ctx context.Context) error,
) {
	jobCtx, cancelJob := context.WithTimeout(ctx, deadline)
	ticker := time.NewTicker(stopPollInterval)
	defer ticker.Stop()
	defer cancelJob()

	jobErrCh := make(chan error)

	retryCall := func(f func() error) error {
		if err := retry.Call(retry.CallArgs{
			Attempts: 5,
			Delay:    time.Second * 30,
			Func:     f,
			Clock:    clock.WallClock,
		}); err != nil {
			return err
		}
		return nil
	}

	go func() {
		if err := j.store.SetJobRunning(ctx, id); err != nil {
			jobErrCh <- fmt.Errorf("failed to set job running, job not starting: %w", err)
			return
		}
		err := job(jobCtx)
		jobErrCh <- err
	}()

	for {
		select {
		case <-jobCtx.Done():
			switch err := jobCtx.Err(); err {
			case context.Canceled:
				if err := retryCall(func() error { return j.store.SetJobFailed(ctx, id, context.Canceled) }); err != nil {
					zapctx.Error(ctx, "failed to set job failed", zap.Error(err))
				}
			case context.DeadlineExceeded:
				if err := retryCall(func() error { return j.store.SetJobFailed(ctx, id, context.DeadlineExceeded) }); err != nil {
					zapctx.Error(ctx, "failed to set job failed", zap.Error(err))
				}
			}
			return
		case err := <-jobErrCh:
			if err != nil {
				if err := retryCall(func() error { return j.store.SetJobFailed(ctx, id, err) }); err != nil {
					zapctx.Error(ctx, "failed to set job failed", zap.Error(err))
				}
			} else {
				if err := retryCall(func() error { return j.store.SetJobSuccessful(ctx, id) }); err != nil {
					zapctx.Error(ctx, "failed to set job successful", zap.Error(err))
				}
			}
			return
		case <-ticker.C:
			shouldStop, err := j.store.GetJobStopSignal(ctx, id)
			if err != nil {
				// If we fail to get the stop signal for any reason, we do a best
				// effort (as the db probably is died on us), so we stop the job,
				// and hope our status setters on context cancellation
				// do eventually write the correct status.
				cancelJob()
				continue
			}

			if shouldStop {
				cancelJob()
				continue
			}
		}
	}
}
