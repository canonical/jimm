// Copyright 2026 Canonical.

package mocks

import (
	"context"

	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/jimm/jobs"
	"github.com/canonical/jimm/v3/pkg/api/params"
)

type JobManager struct {
	GetJobInfo_                            func(ctx context.Context, jobID int64) (jobs.JobInfo, error)
	GetActiveBootstrapStatusForController_ func(ctx context.Context, controllerName string) (*params.BootstrapJobStatus, error)
	GetUpgradeToStatusForModel_            func(ctx context.Context, modelUUID string) (*params.UpgradeToJobStatus, error)
	ListJobs_                              func(ctx context.Context, req params.ListJobsRequest) (params.ListJobsResponse, error)
}

func (j *JobManager) GetJobInfo(ctx context.Context, jobID int64) (jobs.JobInfo, error) {
	if j.GetJobInfo_ == nil {
		return jobs.JobInfo{}, errors.New("not implemented")
	}
	return j.GetJobInfo_(ctx, jobID)
}

func (j *JobManager) GetActiveBootstrapStatusForController(ctx context.Context, controllerName string) (*params.BootstrapJobStatus, error) {
	if j.GetActiveBootstrapStatusForController_ == nil {
		return nil, errors.New("not implemented")
	}
	return j.GetActiveBootstrapStatusForController_(ctx, controllerName)
}

func (j *JobManager) GetUpgradeToStatusForModel(ctx context.Context, modelUUID string) (*params.UpgradeToJobStatus, error) {
	if j.GetUpgradeToStatusForModel_ == nil {
		return nil, errors.New("not implemented")
	}
	return j.GetUpgradeToStatusForModel_(ctx, modelUUID)
}

func (j *JobManager) ListJobs(ctx context.Context, req params.ListJobsRequest) (params.ListJobsResponse, error) {
	if j.ListJobs_ == nil {
		return params.ListJobsResponse{}, errors.New("not implemented")
	}
	return j.ListJobs_(ctx, req)
}
