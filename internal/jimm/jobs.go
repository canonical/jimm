// Copyright 2024 Canonical Ltd.

package jimm

import (
	"context"
	"fmt"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/canonical/jimm/api/params"
	"github.com/canonical/jimm/internal/errors"
)

// GetJobListParams constructs the river.JobListParams based on params.FindJobsRequest and state
func GetJobListParams(req params.FindJobsRequest, state rivertype.JobState) *river.JobListParams {
	sortOrder := river.SortOrderDesc
	if req.SortAsc {
		sortOrder = river.SortOrderAsc
	}
	first := max(1, min(req.Limit, 10000))
	return river.NewJobListParams().First(first).OrderBy(river.JobListOrderByTime, sortOrder).State(state)
}

// GetJobs returns a job based on a specific state and filters
func (j *JIMM) GetJobs(ctx context.Context, req params.FindJobsRequest, state rivertype.JobState) (jobs []params.Job, err error) {
	const op = errors.Op("jimm.GetJobs")
	jobsList, err := j.River.Client.JobList(ctx, GetJobListParams(req, state))
	if err != nil {
		return nil, errors.E(op, fmt.Sprintf("failed to read %s jobs from river db, err: %s", state, err))
	}
	jobs = make([]params.Job, len(jobsList))
	for i, job := range jobsList {
		jobs[i] = params.ConvertJobRowToJob(job)
	}
	return jobs, nil
}

// FindJobs returns a params.Jobs with Failed, Cancelled, and Completed jobs filtered based on the FindJobsRequest
func (j *JIMM) FindJobs(ctx context.Context, req params.FindJobsRequest) (Jobs params.Jobs, err error) {
	if req.IncludeFailed {
		if Jobs.FailedJobs, err = j.GetJobs(ctx, req, rivertype.JobStateDiscarded); err != nil {
			return params.Jobs{}, err
		}
	}
	if req.IncludeCancelled {
		if Jobs.CancelledJobs, err = j.GetJobs(ctx, req, rivertype.JobStateCancelled); err != nil {
			return params.Jobs{}, err
		}
	}
	if req.IncludeCompleted {
		if Jobs.CompletedJobs, err = j.GetJobs(ctx, req, rivertype.JobStateCompleted); err != nil {
			return params.Jobs{}, err
		}
	}
	return Jobs, nil
}
