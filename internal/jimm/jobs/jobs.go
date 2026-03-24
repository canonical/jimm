// Copyright 2025 Canonical.

package jobs

import (
	"context"
	"fmt"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/canonical/jimm/v3/internal/errors"
	apiparams "github.com/canonical/jimm/v3/pkg/api/params"
)

const defaultListJobsCount = 100
const maxListJobsCount = 10_000

// JobQuerier defines the interface for querying and managing jobs in JIMM.
type JobQuerier interface {
	GetJobInfo(ctx context.Context, jobID int64) (*rivertype.JobRow, error)
	ListJobs(ctx context.Context, params *river.JobListParams) (*river.JobListResult, error)
	CancelJob(ctx context.Context, jobID int64) (*rivertype.JobRow, error)
}

type JobManager struct {
	jobQuerier JobQuerier
}

// NewJobManager returns a new job manager that provides management
// abilities for asynchronous jobs within JIMM.
func NewJobManager(jobQuerier JobQuerier) (*JobManager, error) {
	if jobQuerier == nil {
		return nil, errors.New("job querier cannot be nil")

	}
	return &JobManager{jobQuerier}, nil
}

func (j *JobManager) GetJobInfo(ctx context.Context, jobID int64) (JobInfo, error) {
	jobRow, err := j.jobQuerier.GetJobInfo(ctx, jobID)
	if err != nil {
		return JobInfo{}, err
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
		Status:         toJobStatus(jobRow.State),
		Kind:           jobRow.Kind,
		CurrentAttempt: jobRow.Attempt,
		MaxAttempts:    jobRow.MaxAttempts,
		FinishedAt:     jobRow.FinalizedAt,
		Errors:         jobErrors,
	}, nil
}

// ListJobs returns a list of jobs based on the provided parameters. It converts the API parameters to the internal river job query parameters and retrieves the job list from the job querier.
func (j *JobManager) ListJobs(ctx context.Context, req apiparams.ListJobsRequest) (apiparams.ListJobsResponse, error) {
	riverStates, err := convertJobStates(req.Statuses)
	if err != nil {
		return apiparams.ListJobsResponse{}, err
	}
	// Set default count if not provided
	count := req.Count
	if count <= 0 {
		count = defaultListJobsCount
	}
	if count > maxListJobsCount {
		return apiparams.ListJobsResponse{}, errors.E(errors.CodeBadRequest, fmt.Sprintf("count must be between 1 and %d.", maxListJobsCount))
	}

	p := river.NewJobListParams().First(count)

	// Only add filters if they are specified
	if len(req.Kinds) > 0 {
		p = p.Kinds(req.Kinds...)
	}
	if len(riverStates) > 0 {
		p = p.States(riverStates...)
	}

	// Handle pagination cursor
	if req.Cursor != "" {
		cursor := &river.JobListCursor{}
		if err := cursor.UnmarshalText([]byte(req.Cursor)); err != nil {
			return apiparams.ListJobsResponse{}, errors.E("invalid cursor: %w", err)
		}
		p = p.After(cursor)
	}

	jobListResult, err := j.jobQuerier.ListJobs(ctx, p)
	if err != nil {
		return apiparams.ListJobsResponse{}, err
	}

	jobs := make([]apiparams.ListJobInfo, len(jobListResult.Jobs))
	for i, job := range jobListResult.Jobs {
		jobs[i] = apiparams.ListJobInfo{
			ID:          job.ID,
			Status:      toJobStatus(job.State),
			Kind:        job.Kind,
			MaxAttempts: job.MaxAttempts,
			Attempt:     job.Attempt,
		}
	}

	// Get next cursor if available
	var nextCursor string
	if jobListResult.LastCursor != nil {
		cursorBytes, err := jobListResult.LastCursor.MarshalText()
		if err != nil {
			return apiparams.ListJobsResponse{}, errors.E("failed to marshal cursor: %w", err)
		}
		nextCursor = string(cursorBytes)
	}

	return apiparams.ListJobsResponse{
		Jobs:       jobs,
		NextCursor: nextCursor,
	}, nil
}

func convertJobStates(statuses []apiparams.JobStatus) ([]rivertype.JobState, error) {
	// If statuses is empty, return empty slice to get all statuses
	if len(statuses) == 0 {
		return []rivertype.JobState{}, nil
	}

	var riverStates []rivertype.JobState
	for _, status := range statuses {
		// Skip unknown/empty statuses
		if status == "" || status == apiparams.StatusUnknown {
			continue
		}

		switch status {
		case apiparams.StatusRunning:
			riverStates = append(riverStates, rivertype.JobStateRunning)
		case apiparams.StatusFailed:
			riverStates = append(riverStates, rivertype.JobStateDiscarded, rivertype.JobStateCancelled)
		case apiparams.StatusSuccessful:
			riverStates = append(riverStates, rivertype.JobStateCompleted)
		case apiparams.StatusPending:
			riverStates = append(riverStates, rivertype.JobStateAvailable, rivertype.JobStateScheduled)
		default:
			return nil, errors.E(errors.CodeBadRequest, fmt.Sprintf("invalid job status: %s", status))
		}
	}

	return riverStates, nil
}

// toJobStatus converts a rivertype.JobState to a params.JobStatus.
func toJobStatus(state rivertype.JobState) apiparams.JobStatus {
	switch state {
	case rivertype.JobStateCompleted:
		return apiparams.StatusSuccessful
	case rivertype.JobStateRunning:
		return apiparams.StatusRunning
	case rivertype.JobStateCancelled, rivertype.JobStateDiscarded:
		return apiparams.StatusFailed
	case rivertype.JobStateAvailable, rivertype.JobStatePending, rivertype.JobStateScheduled, rivertype.JobStateRetryable:
		return apiparams.StatusPending
	default:
		return apiparams.StatusUnknown
	}
}
