// Copyright 2025 Canonical.

package jobs

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/rivertypes"
	apiparams "github.com/canonical/jimm/v3/pkg/api/params"
)

const maxListJobsCount = 10_000

var activeJobStates = []rivertype.JobState{
	rivertype.JobStateAvailable,
	rivertype.JobStatePending,
	rivertype.JobStateRunning,
	rivertype.JobStateRetryable,
	rivertype.JobStateScheduled,
}

var finalizedUpgradeToJobStates = []rivertype.JobState{
	rivertype.JobStateCancelled,
	rivertype.JobStateCompleted,
	rivertype.JobStateDiscarded,
}

const (
	// UpgradeToModelStatusProgress indicates an upgrade-to job is currently in progress for the model.
	// It maps to in-progress job states: available, pending, running, retryable, scheduled.
	UpgradeToModelStatusProgress = "progress"
	// UpgradeToModelStatusError indicates an upgrade-to job has encountered an error for the model.
	// It maps to error job states: cancelled, discarded.
	UpgradeToModelStatusError = "error"
	// UpgradeToModelStatusCompleted indicates the latest upgrade-to job completed successfully.
	// It maps to the completed job state.
	UpgradeToModelStatusCompleted = "completed"
)

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

// GetActiveBootstrapStatusForController returns the status of the active bootstrap
// job for the specified controller, if one exists.
func (j *JobManager) GetActiveBootstrapStatusForController(ctx context.Context, controllerName string) (*apiparams.BootstrapJobStatus, error) {
	jobListResult, err := j.jobQuerier.ListJobs(
		ctx,
		river.NewJobListParams().
			Kinds(rivertypes.BootstrapJobKind).
			First(1).
			States(activeJobStates...).
			Where(
				"metadata->>'controller-name' = @controller_name",
				river.NamedArgs{"controller_name": controllerName},
			),
	)
	if err != nil {
		return nil, err
	}
	if len(jobListResult.Jobs) == 0 {
		return nil, nil
	}
	return &apiparams.BootstrapJobStatus{
		Bootstrap: toJobDetail(jobListResult.Jobs[0]),
	}, nil
}

// GetUpgradeToStatusForModel returns the status of the current or most recent
// finalized upgrade-to job for the specified model.
func (j *JobManager) GetUpgradeToStatusForModel(ctx context.Context, modelUUID string) (*apiparams.UpgradeToJobStatus, error) {
	job, err := j.findUpgradeToJob(ctx, modelUUID)
	if err != nil {
		return nil, err
	}
	if job == nil {
		return nil, nil
	}

	var output rivertypes.UpgradeToOutput
	rawOutput := job.Output()
	if len(rawOutput) != 0 {
		if err := json.Unmarshal(rawOutput, &output); err != nil {
			return nil, fmt.Errorf("failed to decode upgrade-to output: %w", err)
		}
	}

	return &apiparams.UpgradeToJobStatus{
		Detail: toJobDetail(job),
		Info:   output.Info,
	}, nil
}

// ListUpgradeToJobsForModels returns a lightweight per-model status for the most
// relevant upgrade-to supervisor job for each requested model.
// Active jobs are loaded first so in-flight work takes precedence, then the most
// recent finalized jobs fill in any remaining models.
func (j *JobManager) ListUpgradeToJobsForModels(ctx context.Context, modelUUIDs []string) (map[string]string, error) {
	jobsByModelUUID := make(map[string]string, len(modelUUIDs))
	if len(modelUUIDs) == 0 {
		return jobsByModelUUID, nil
	}

	requestedModelUUIDs := make(map[string]struct{}, len(modelUUIDs))
	for _, modelUUID := range modelUUIDs {
		requestedModelUUIDs[modelUUID] = struct{}{}
	}

	activeJobListResult, err := j.jobQuerier.ListJobs(
		ctx,
		river.NewJobListParams().
			Kinds(rivertypes.UpgradeToJobKind).
			First(maxListJobsCount).
			States(activeJobStates...).
			OrderBy(river.JobListOrderByTime, river.SortOrderDesc),
	)
	if err != nil {
		return nil, err
	}

	for _, job := range activeJobListResult.Jobs {
		if job == nil {
			continue
		}
		modelUUID, err := requestedUpgradeToModelUUID(job, requestedModelUUIDs)
		if err != nil {
			return nil, err
		}
		if modelUUID == "" {
			continue
		}
		jobsByModelUUID[modelUUID] = UpgradeToModelStatusProgress
	}

	finalizedJobListResult, err := j.jobQuerier.ListJobs(
		ctx,
		river.NewJobListParams().
			Kinds(rivertypes.UpgradeToJobKind).
			First(maxListJobsCount).
			States(finalizedUpgradeToJobStates...).
			OrderBy(river.JobListOrderByTime, river.SortOrderDesc),
	)
	if err != nil {
		return nil, err
	}

	for _, job := range finalizedJobListResult.Jobs {
		if job == nil {
			continue
		}
		modelUUID, err := requestedUpgradeToModelUUID(job, requestedModelUUIDs)
		if err != nil {
			return nil, err
		}
		if modelUUID == "" {
			continue
		}
		if _, ok := jobsByModelUUID[modelUUID]; ok {
			continue
		}

		switch job.State {
		case rivertype.JobStateCancelled, rivertype.JobStateDiscarded:
			jobsByModelUUID[modelUUID] = UpgradeToModelStatusError
		case rivertype.JobStateCompleted:
			jobsByModelUUID[modelUUID] = UpgradeToModelStatusCompleted
		case rivertype.JobStateAvailable, rivertype.JobStatePending, rivertype.JobStateRunning, rivertype.JobStateRetryable, rivertype.JobStateScheduled:
			jobsByModelUUID[modelUUID] = UpgradeToModelStatusProgress
		}
	}

	return jobsByModelUUID, nil
}

// requestedUpgradeToModelUUID checks if the job has metadata indicating it is an upgrade-to job for one of the requested
// model UUIDs. If so, it returns that model UUID; otherwise, it returns an empty string.
func requestedUpgradeToModelUUID(job *rivertype.JobRow, requestedModelUUIDs map[string]struct{}) (string, error) {
	var metadata rivertypes.JobModelUUIDMetadata
	if err := json.Unmarshal(job.Metadata, &metadata); err != nil {
		return "", fmt.Errorf("failed to decode upgrade-to metadata: %w", err)
	}
	if metadata.ModelUUID == "" {
		return "", nil
	}
	if _, ok := requestedModelUUIDs[metadata.ModelUUID]; !ok {
		return "", nil
	}
	return metadata.ModelUUID, nil
}

// findUpgradeToJob finds the current active or most recently finalized
// upgrade-to supervisor job for the specified model.
//
// This uses two queries so an in-flight supervisor job is preferred over any
// older finalized job. If no active job exists, it falls back to the most
// recently finalized supervisor so callers can still see the last terminal
// upgrade-to status.
func (j *JobManager) findUpgradeToJob(ctx context.Context, modelUUID string) (*rivertype.JobRow, error) {
	activeJobs, err := j.jobQuerier.ListJobs(
		ctx,
		river.NewJobListParams().
			Kinds(rivertypes.UpgradeToJobKind).
			First(1).
			States(activeJobStates...).
			Where(
				"metadata->>'model-uuid' = @model_uuid",
				river.NamedArgs{"model_uuid": modelUUID},
			),
	)
	if err != nil {
		return nil, err
	}
	if len(activeJobs.Jobs) > 0 {
		return activeJobs.Jobs[0], nil
	}

	finalizedJobs, err := j.jobQuerier.ListJobs(
		ctx,
		river.NewJobListParams().
			Kinds(rivertypes.UpgradeToJobKind).
			First(1).
			States(finalizedUpgradeToJobStates...).
			Where(
				"metadata->>'model-uuid' = @model_uuid",
				river.NamedArgs{"model_uuid": modelUUID},
			).
			OrderBy(river.JobListOrderByTime, river.SortOrderDesc),
	)
	if err != nil {
		return nil, err
	}
	if len(finalizedJobs.Jobs) == 0 {
		return nil, nil
	}

	return finalizedJobs.Jobs[0], nil
}

func toJobDetail(jobRow *rivertype.JobRow) apiparams.JobDetail {
	var jobErrors []apiparams.JobAttemptError
	for _, err := range jobRow.Errors {
		jobErrors = append(jobErrors, apiparams.JobAttemptError{
			Attempt: err.Attempt,
			At:      err.At,
			Error:   err.Error,
		})
	}

	return apiparams.JobDetail{
		State:       string(jobRow.State),
		Attempt:     jobRow.Attempt,
		MaxAttempts: jobRow.MaxAttempts,
		AttemptedAt: jobRow.AttemptedAt,
		FinalizedAt: jobRow.FinalizedAt,
		Errors:      jobErrors,
	}
}
