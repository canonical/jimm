// Copyright 2025 Canonical.

// Package jobtracker provides a way to run routines in one instance of JIMM and track them in another.
// That is, their status can be checked and they can be stopped.
package jobtracker

import (
	"context"
	goerr "errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/juju/zaputil/zapctx"
	"go.uber.org/zap"

	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
)

const minimumRefreshIntervalseconds = 5

// JobIdContextKey is a context key for storing job IDs.
type JobIdContextKey struct{}

// Store defines the interface for tracking the lifecycle and status of jobs.
// It provides methods to add a new job, update its status (running, successful, or failed),
// and retrieve a stop signal for a specific job.
type Store interface {
	AddJob(ctx context.Context, jobType string) (uuid.UUID, error)
	GetJob(ctx context.Context, job *dbmodel.JobTrackerEntry) (err error)
	StopJob(ctx context.Context, jobId uuid.UUID) (err error)
	SetJobRunning(ctx context.Context, jobId uuid.UUID) error
	SetJobSuccessful(ctx context.Context, jobId uuid.UUID) error
	SetJobFailed(ctx context.Context, jobId uuid.UUID, jobErr error) error
	GetJobStopSignal(ctx context.Context, jobId uuid.UUID) (stopSignal bool, err error)
}

// Tracker manages job tracking operations using a provided JobTrackerStore.
// It periodically performs tasks based on the specified refreshInterval duration.
type Tracker struct {
	store           Store
	refreshInterval time.Duration
}

// NewJobTracker creates and returns a new Tracker instance using the provided JobTrackerStore and refreshInterval.
// refreshInterval is the interval between successive checks for job status.
// It returns an error if the store is nil or if refreshInterval is not greater than zero.
func NewJobTracker(store Store, refreshInterval time.Duration) (*Tracker, error) {
	if store == nil {
		return nil, goerr.New("store cannot be nil")
	}
	if refreshInterval < minimumRefreshIntervalseconds*time.Second {
		return nil, fmt.Errorf("refreshInterval must be greater than %d seconds", minimumRefreshIntervalseconds)
	}

	return &Tracker{
		refreshInterval: refreshInterval,
		store:           store,
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
	jobCtx = ContextWithJobId(jobCtx, id)
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
	ticker := time.NewTicker(j.refreshInterval)
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

// GetJob retrieves a Job via its jobId.
func (j *Tracker) GetJob(ctx context.Context, jobId uuid.UUID) (dbmodel.JobTrackerEntry, error) {
	const op = errors.Op("jimm.GetJobStatus")

	job := dbmodel.JobTrackerEntry{JobID: jobId}
	err := j.store.GetJob(ctx, &job)
	if err != nil {
		return job, errors.E(op, "failed to get job status", err)
	}

	return job, nil
}

// StopJob stops a job by its ID.
func (j *Tracker) StopJob(ctx context.Context, jobId uuid.UUID) error {
	const op = errors.Op("jimm.StopJob")

	if err := j.store.StopJob(ctx, jobId); err != nil {
		return errors.E(op, "failed to stop job", err)
	}

	return nil
}

// ContextWithJobId adds the job ID to the context for later retrieval.
func ContextWithJobId(ctx context.Context, jobId uuid.UUID) context.Context {
	return context.WithValue(ctx, JobIdContextKey{}, jobId)
}

// JobIdFromContext retrieves the job ID from the context.
func JobIdFromContext(ctx context.Context) (uuid.UUID, bool) {
	jobId, ok := ctx.Value(JobIdContextKey{}).(uuid.UUID)
	return jobId, ok
}
