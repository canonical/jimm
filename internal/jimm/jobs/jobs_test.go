// Copyright 2025 Canonical.

package jobs

import (
	"context"
	"testing"
	"time"

	"github.com/frankban/quicktest"
	"github.com/riverqueue/river/rivertype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/canonical/jimm/v3/internal/errors"
)

type testDeps struct {
	jobManager *JobManager
	jobQuerier *MockJobQuerier
}

func setupDeps(c *quicktest.C) testDeps {
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
	c := quicktest.New(t)
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
	c := quicktest.New(t)
	deps := setupDeps(c)

	ctx := context.Background()
	jobID := int64(123)

	deps.jobQuerier.EXPECT().GetJobInfo(gomock.Any(), int64(123)).
		Return(nil, errors.New("query error"))

	_, err := deps.jobManager.GetJobInfo(ctx, jobID)
	c.Assert(err, quicktest.IsNotNil)
	c.Assert(err, quicktest.ErrorMatches, "query error")
}

func TestNewJobManager_NilQuerier(t *testing.T) {
	c := quicktest.New(t)

	_, err := NewJobManager(nil)
	c.Assert(err, quicktest.IsNotNil)
}
