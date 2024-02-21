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

// GetJobListParams constructs the river.JobListParams based on params.ViewJobsRequest and state
func GetJobListParams(req params.ViewJobsRequest, state rivertype.JobState) *river.JobListParams {
	sortOrder := river.SortOrderDesc
	if req.SortAsc {
		sortOrder = river.SortOrderAsc
	}
	first := max(1, min(req.Limit, 10000))
	return river.NewJobListParams().First(first).OrderBy(river.JobListOrderByTime, sortOrder).State(state)
}

// GetJobs returns a job based on a specific state and filters
func (j *JIMM) GetJobs(ctx context.Context, req params.ViewJobsRequest, state rivertype.JobState) (jobs []rivertype.JobRow, err error) {
	const op = errors.Op("jimm.GetJobs")
	jobsList, err := j.River.Client.JobList(ctx, GetJobListParams(req, state))
	if err != nil {
		return nil, errors.E(op, fmt.Sprintf("failed to read %s jobs from river db, err: %s", state, err))
	}
	jobs = make([]rivertype.JobRow, len(jobsList))
	for i, job := range jobsList {
		jobs[i] = *job
	}
	return jobs, nil
}

// ViewJobs returns a params.RiverJobs with Failed, Cancelled, and Completed jobs filtered based on the ViewJobsRequest
func (j *JIMM) ViewJobs(ctx context.Context, req params.ViewJobsRequest) (riverJobs params.RiverJobs, err error) {
	if req.IncludeFailed {
		if riverJobs.FailedJobs, err = j.GetJobs(ctx, req, rivertype.JobStateDiscarded); err != nil {
			return params.RiverJobs{}, err
		}
	}
	if req.IncludeCancelled {
		if riverJobs.CancelledJobs, err = j.GetJobs(ctx, req, rivertype.JobStateCancelled); err != nil {
			return params.RiverJobs{}, err
		}
	}
	if req.IncludeCompleted {
		if riverJobs.CompletedJobs, err = j.GetJobs(ctx, req, rivertype.JobStateCompleted); err != nil {
			return params.RiverJobs{}, err
		}
	}
	return riverJobs, nil
}
