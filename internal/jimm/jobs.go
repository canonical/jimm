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

func (j *JIMM) ViewJobs(ctx context.Context, req params.ViewJobsRequest) (riverJobs params.RiverJobs, err error) {
	const op = errors.Op("jimm.ViewJobs")
	client := j.River.Client
	req.Limit = min(req.Limit, 10000) // count greater than 10_000 makes river panics, which kills jimm.

	getJobs := func(state rivertype.JobState) (jobs []rivertype.JobRow, err error) {
		sortOrder := river.SortOrderDesc
		if req.SortAsc {
			sortOrder = river.SortOrderAsc
		}
		jobsList, err := client.JobList(
			ctx,
			river.NewJobListParams().
				State(state).
				OrderBy(river.JobListOrderByTime, sortOrder).
				First(req.Limit),
		)
		if err != nil {
			return make([]rivertype.JobRow, 0), errors.E(op, fmt.Sprintf("failed to read %s jobs from river db, err: %s", state, err))
		}
		jobs = make([]rivertype.JobRow, len(jobsList))
		for i, job := range jobsList {
			jobs[i] = *job
		}
		return jobs, nil
	}

	if req.IncludeFailed {
		if riverJobs.FailedJobs, err = getJobs(rivertype.JobStateDiscarded); err != nil {
			return params.RiverJobs{}, err
		}
	}
	if req.IncludeCancelled {
		if riverJobs.CancelledJobs, err = getJobs(rivertype.JobStateCancelled); err != nil {
			return params.RiverJobs{}, err
		}
	}
	if req.IncludeCompleted {
		if riverJobs.CompletedJobs, err = getJobs(rivertype.JobStateCompleted); err != nil {
			return params.RiverJobs{}, err
		}
	}
	return riverJobs, nil
}
