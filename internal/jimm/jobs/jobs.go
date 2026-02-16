// Copyright 2026 Canonical.

package jobs

import (
	"context"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/canonical/jimm/v3/internal/errors"
)

// JobQuerier defines the interface for querying and managing jobs in JIMM.
type JobQuerier interface {
	GetJobInfo(ctx context.Context, jobID int64) (*rivertype.JobRow, error)
	ListJobs(ctx context.Context, params *river.JobListParams) (*river.JobListResult, error)
	CancelJob(ctx context.Context, jobID int64) (*rivertype.JobRow, error)
}

type jobManager struct {
	jobQuerier JobQuerier
}

// NewJobManager returns a new job manager that provides management
// abilities for asynchronous jobs within JIMM.
func NewJobManager(jobQuerier JobQuerier) (*jobManager, error) {
	if jobQuerier == nil {
		return nil, errors.E("job querier cannot be nil")

	}
	return &jobManager{jobQuerier}, nil
}

func (j *jobManager) GetJobInfo(ctx context.Context, jobID int64) (JobInfo, error) {
	jobRow, err := j.jobQuerier.GetJobInfo(ctx, jobID)
	if err != nil {
		return JobInfo{}, errors.E(err)
	}
	var jobErrors []JobError
	for _, err := range jobRow.Errors {
		jobErrors = append(jobErrors, JobError{
			Error:   err.Error,
			At:      err.At,
			Attempt: err.Attempt,
		})
	}
	return JobInfo{
		ID:             jobRow.ID,
		Status:         string(jobRow.State),
		Kind:           jobRow.Kind,
		CurrentAttempt: jobRow.Attempt,
		MaxAttempts:    jobRow.MaxAttempts,
		FinishedAt:     jobRow.FinalizedAt,
		Errors:         jobErrors,
	}, nil
}
