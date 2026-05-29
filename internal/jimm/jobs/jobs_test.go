// Copyright 2025 Canonical.

package jobs

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	qt "github.com/frankban/quicktest"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/rivertypes"
	apiparams "github.com/canonical/jimm/v3/pkg/api/params"
)

type testDeps struct {
	jobManager *JobManager
	jobQuerier *MockJobQuerier
}

func setupDeps(c *qt.C) testDeps {
	ctrl := gomock.NewController(c)
	jobQuerier := NewMockJobQuerier(ctrl)
	c.Cleanup(ctrl.Finish)

	manager, err := NewJobManager(jobQuerier)
	require.NoError(c, err)

	deps := testDeps{
		jobManager: manager,
		jobQuerier: jobQuerier,
	}

	return deps
}

func TestGetActiveBootstrapStatusForController_Success(t *testing.T) {
	c := qt.New(t)
	deps := setupDeps(c)

	ctx := context.Background()
	controllerName := "controller-name"
	attemptedAt := time.Now().Add(-time.Minute)
	finalizedAt := time.Now()

	deps.jobQuerier.EXPECT().ListJobs(gomock.Any(), gomock.Any()).Return(&river.JobListResult{Jobs: []*rivertype.JobRow{{
		ID:          123,
		State:       rivertype.JobStateRunning,
		Attempt:     2,
		MaxAttempts: 5,
		AttemptedAt: &attemptedAt,
		FinalizedAt: &finalizedAt,
		Errors: []rivertype.AttemptError{{
			Error:   "test error",
			At:      finalizedAt,
			Attempt: 1,
		}},
	}}}, nil)

	status, err := deps.jobManager.GetActiveBootstrapStatusForController(ctx, controllerName)
	c.Assert(err, qt.IsNil)
	c.Assert(status, qt.DeepEquals, &apiparams.BootstrapJobStatus{
		Bootstrap: apiparams.JobDetail{
			State:       string(rivertype.JobStateRunning),
			Attempt:     2,
			MaxAttempts: 5,
			AttemptedAt: &attemptedAt,
			FinalizedAt: &finalizedAt,
			Errors: []apiparams.JobAttemptError{{
				Attempt: 1,
				At:      finalizedAt,
				Error:   "test error",
			}},
		},
	})
}

func TestGetActiveBootstrapStatusForController_NoJob(t *testing.T) {
	c := qt.New(t)
	deps := setupDeps(c)

	deps.jobQuerier.EXPECT().ListJobs(gomock.Any(), gomock.Any()).Return(&river.JobListResult{}, nil)

	status, err := deps.jobManager.GetActiveBootstrapStatusForController(context.Background(), "controller-name")
	c.Assert(err, qt.IsNil)
	c.Assert(status, qt.IsNil)
}

func TestNewJobManager_NilQuerier(t *testing.T) {
	c := qt.New(t)

	_, err := NewJobManager(nil)
	c.Assert(err, qt.IsNotNil)
}

func TestGetUpgradeToStatusForModel_NoJob(t *testing.T) {
	c := qt.New(t)
	deps := setupDeps(c)

	ctx := context.Background()
	modelUUID := "model-uuid"

	deps.jobQuerier.EXPECT().ListJobs(gomock.Any(), gomock.Any()).Return(&river.JobListResult{}, nil)
	deps.jobQuerier.EXPECT().ListJobs(gomock.Any(), gomock.Any()).Return(&river.JobListResult{}, nil)

	status, err := deps.jobManager.GetUpgradeToStatusForModel(ctx, modelUUID)
	c.Assert(err, qt.IsNil)
	c.Assert(status, qt.IsNil)
}

func TestGetUpgradeToStatusForModel_ActiveQueryError(t *testing.T) {
	c := qt.New(t)
	deps := setupDeps(c)

	ctx := context.Background()

	deps.jobQuerier.EXPECT().ListJobs(gomock.Any(), gomock.Any()).Return(nil, errors.New("query error"))

	status, err := deps.jobManager.GetUpgradeToStatusForModel(ctx, "model-uuid")
	c.Assert(err, qt.ErrorMatches, "query error")
	c.Assert(status, qt.IsNil)
}

func TestListUpgradeToJobsForModels_Success(t *testing.T) {
	c := qt.New(t)
	deps := setupDeps(c)

	metadataOne, err := json.Marshal(rivertypes.JobModelUUIDMetadata{ModelUUID: "model-uuid-1"})
	c.Assert(err, qt.IsNil)
	metadataTwo, err := json.Marshal(rivertypes.JobModelUUIDMetadata{ModelUUID: "model-uuid-2"})
	c.Assert(err, qt.IsNil)
	metadataThree, err := json.Marshal(rivertypes.JobModelUUIDMetadata{ModelUUID: "model-uuid-3"})
	c.Assert(err, qt.IsNil)
	metadataCompleted, err := json.Marshal(rivertypes.JobModelUUIDMetadata{ModelUUID: "model-uuid-2"})
	c.Assert(err, qt.IsNil)

	gomock.InOrder(
		deps.jobQuerier.EXPECT().ListJobs(gomock.Any(), gomock.Any()).Return(&river.JobListResult{Jobs: []*rivertype.JobRow{
			{Metadata: metadataOne, State: rivertype.JobStateRunning},
			{Metadata: metadataThree, State: rivertype.JobStateRunning},
		}}, nil),
		deps.jobQuerier.EXPECT().ListJobs(gomock.Any(), gomock.Any()).Return(&river.JobListResult{Jobs: []*rivertype.JobRow{
			{Metadata: metadataTwo, State: rivertype.JobStateDiscarded},
			{Metadata: metadataCompleted, State: rivertype.JobStateCompleted},
		}}, nil),
	)

	jobsByModel, err := deps.jobManager.ListUpgradeToJobsForModels(context.Background(), []string{"model-uuid-1", "model-uuid-2"})
	c.Assert(err, qt.IsNil)
	c.Assert(jobsByModel, qt.DeepEquals, map[string]string{
		"model-uuid-1": UpgradeToModelStatusProgress,
		"model-uuid-2": UpgradeToModelStatusError,
	})
}

func TestListUpgradeToJobsForModels_CompletedJobSuppressesOlderError(t *testing.T) {
	c := qt.New(t)
	deps := setupDeps(c)

	metadata, err := json.Marshal(rivertypes.JobModelUUIDMetadata{ModelUUID: "model-uuid-1"})
	c.Assert(err, qt.IsNil)

	gomock.InOrder(
		deps.jobQuerier.EXPECT().ListJobs(gomock.Any(), gomock.Any()).Return(&river.JobListResult{}, nil),
		deps.jobQuerier.EXPECT().ListJobs(gomock.Any(), gomock.Any()).Return(&river.JobListResult{Jobs: []*rivertype.JobRow{
			{Metadata: metadata, State: rivertype.JobStateCompleted},
			{Metadata: metadata, State: rivertype.JobStateDiscarded},
		}}, nil),
	)

	jobsByModel, err := deps.jobManager.ListUpgradeToJobsForModels(context.Background(), []string{"model-uuid-1"})
	c.Assert(err, qt.IsNil)
	c.Assert(jobsByModel, qt.DeepEquals, map[string]string{
		"model-uuid-1": UpgradeToModelStatusCompleted,
	})
}

func TestListUpgradeToJobsForModels_ActiveJobSuppressesOlderError(t *testing.T) {
	c := qt.New(t)
	deps := setupDeps(c)

	metadata, err := json.Marshal(rivertypes.JobModelUUIDMetadata{ModelUUID: "model-uuid-1"})
	c.Assert(err, qt.IsNil)

	gomock.InOrder(
		deps.jobQuerier.EXPECT().ListJobs(gomock.Any(), gomock.Any()).Return(&river.JobListResult{Jobs: []*rivertype.JobRow{
			{Metadata: metadata, State: rivertype.JobStateRunning},
		}}, nil),
		deps.jobQuerier.EXPECT().ListJobs(gomock.Any(), gomock.Any()).Return(&river.JobListResult{Jobs: []*rivertype.JobRow{
			{Metadata: metadata, State: rivertype.JobStateDiscarded},
		}}, nil),
	)

	jobsByModel, err := deps.jobManager.ListUpgradeToJobsForModels(context.Background(), []string{"model-uuid-1"})
	c.Assert(err, qt.IsNil)
	c.Assert(jobsByModel, qt.DeepEquals, map[string]string{
		"model-uuid-1": UpgradeToModelStatusProgress,
	})
}

func TestListUpgradeToJobsForModels_QueryError(t *testing.T) {
	c := qt.New(t)
	deps := setupDeps(c)

	gomock.InOrder(
		deps.jobQuerier.EXPECT().ListJobs(gomock.Any(), gomock.Any()).Return(&river.JobListResult{}, nil),
		deps.jobQuerier.EXPECT().ListJobs(gomock.Any(), gomock.Any()).Return(nil, errors.New("query error")),
	)

	jobsByModel, err := deps.jobManager.ListUpgradeToJobsForModels(context.Background(), []string{"model-uuid"})
	c.Assert(err, qt.ErrorMatches, "query error")
	c.Assert(jobsByModel, qt.IsNil)
}
