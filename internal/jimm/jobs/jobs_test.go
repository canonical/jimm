// Copyright 2025 Canonical.

package jobs

import (
	"context"
	"testing"
	"time"

	qt "github.com/frankban/quicktest"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/canonical/jimm/v3/internal/errors"
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

func TestGetJobInfo_Success(t *testing.T) {
	c := qt.New(t)
	deps := setupDeps(c)

	ctx := context.Background()
	jobID := int64(123)
	finishedAt := time.Now()

	deps.jobQuerier.EXPECT().GetJobInfo(gomock.Any(), jobID).
		Return(&rivertype.JobRow{
			ID:          jobID,
			Kind:        "test_job",
			State:       rivertype.JobStateCompleted,
			Attempt:     2,
			MaxAttempts: 5,
			FinalizedAt: &finishedAt,
			Errors: []rivertype.AttemptError{
				{Error: "test error", At: time.Now(), Attempt: 1},
			},
		}, nil)

	result, err := deps.jobManager.GetJobInfo(ctx, jobID)
	assert.NoError(t, err)
	assert.Equal(t, jobID, result.ID)
	assert.Equal(t, "test_job", result.Kind)
	assert.Equal(t, 2, result.CurrentAttempt)
	assert.Equal(t, 5, result.MaxAttempts)
	assert.Equal(t, 1, len(result.Errors))
}

func TestGetJobInfo_QueryError(t *testing.T) {
	c := qt.New(t)
	deps := setupDeps(c)

	ctx := context.Background()
	jobID := int64(123)

	deps.jobQuerier.EXPECT().GetJobInfo(gomock.Any(), int64(123)).
		Return(nil, errors.New("query error"))

	_, err := deps.jobManager.GetJobInfo(ctx, jobID)
	c.Assert(err, qt.IsNotNil)
	c.Assert(err, qt.ErrorMatches, "query error")
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
