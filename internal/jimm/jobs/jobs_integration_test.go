// Copyright 2026 Canonical.

package jobs

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	qt "github.com/frankban/quicktest"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverdatabasesql"
	"github.com/riverqueue/river/rivertype"

	"github.com/canonical/jimm/v3/internal/db"
	jimmriver "github.com/canonical/jimm/v3/internal/river"
	"github.com/canonical/jimm/v3/internal/rivertypes"
	"github.com/canonical/jimm/v3/internal/testutils/testdb"
	apiparams "github.com/canonical/jimm/v3/pkg/api/params"
)

const defaultTestTimeout = time.Minute

// successJobArgs is a job type that always succeeds.
type successJobArgs struct {
	Name string
}

// Kind returns the job kind.
func (successJobArgs) Kind() string { return "test-success-job" }

func (successJobArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{MaxAttempts: 1}
}

// successJobWorker is a worker that always succeeds.
type successJobWorker struct {
	river.WorkerDefaults[successJobArgs]
}

func (w *successJobWorker) Work(ctx context.Context, job *river.Job[successJobArgs]) error {
	return nil
}

// failureJobArgs is a job type that always fails.
type failureJobArgs struct {
	Name string
}

// Kind returns the job kind.
func (failureJobArgs) Kind() string { return "test-failure-job" }

func (failureJobArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{MaxAttempts: 1}
}

// failureJobWorker is a worker that always fails with cancelled state.
type failureJobWorker struct {
	river.WorkerDefaults[failureJobArgs]
}

func (w *failureJobWorker) Work(ctx context.Context, job *river.Job[failureJobArgs]) error {
	return &river.JobCancelError{}
}

// upgradeToTestWorker exists only to register the upgrade-to kind with the test
// client. Tests place root jobs on an unworked queue so this worker should not run.
type upgradeToTestWorker struct {
	river.WorkerDefaults[rivertypes.UpgradeToArgs]
}

func (w *upgradeToTestWorker) Work(ctx context.Context, job *river.Job[rivertypes.UpgradeToArgs]) error {
	if strings.HasPrefix(job.Args.Username, "discard-") {
		return fmt.Errorf("discarded upgrade root for %s", job.Args.Username)
	}
	return nil
}

// waitForJobs waits for the specified number of jobs to complete or fail.
// Returns when all jobs have finalized or timeout occurs.
func waitForJobs(c *qt.C, client *river.Client[*sql.Tx], expectedCount int, timeout time.Duration) {
	sub, cancel := client.Subscribe(river.EventKindJobCompleted, river.EventKindJobCancelled, river.EventKindJobFailed)
	defer cancel()

	completed := 0
	timer := time.After(timeout)
	for completed < expectedCount {
		select {
		case <-sub:
			completed++
		case <-timer:
			c.Fatalf("timeout waiting for %d jobs to complete (got %d)", expectedCount, completed)
		}
	}
}

func setupJobsIntegrationTest(c *qt.C) (*JobManager, *river.Client[*sql.Tx]) {
	// Setup database with JIMM and River migrations
	database := &db.Database{
		DB: testdb.PostgresDB(c, time.Now),
	}
	err := database.Migrate(c.Context())
	c.Assert(err, qt.IsNil)

	err = jimmriver.MigrateRiver(c.Context(), database)
	c.Assert(err, qt.IsNil)

	sqlDB, err := database.SqlDB()
	c.Assert(err, qt.IsNil)
	c.Cleanup(func() {
		c.Check(sqlDB.Close(), qt.IsNil)
	})

	// Setup test workers (success and failure)
	workers := river.NewWorkers()
	err = river.AddWorkerSafely(workers, &successJobWorker{})
	c.Assert(err, qt.IsNil)
	err = river.AddWorkerSafely(workers, &failureJobWorker{})
	c.Assert(err, qt.IsNil)
	err = river.AddWorkerSafely(workers, &upgradeToTestWorker{})
	c.Assert(err, qt.IsNil)

	// Start River client
	riverClient, err := river.NewClient(riverdatabasesql.New(sqlDB), &river.Config{
		TestOnly: true,
		Queues: map[string]river.QueueConfig{
			river.QueueDefault: {MaxWorkers: 5},
		},
		Workers: workers,
	})
	c.Assert(err, qt.IsNil)

	c.Assert(riverClient.Start(c.Context()), qt.IsNil)
	c.Cleanup(func() {
		err := riverClient.Stop(context.Background())
		c.Check(err, qt.IsNil)
	})

	// Create wrapped client for JobManager
	wrappedClient := &jimmriver.Client{}
	wrappedClient.SetClient(riverClient)

	// Create JobManager
	jobManager, err := NewJobManager(wrappedClient)
	c.Assert(err, qt.IsNil)

	return jobManager, riverClient
}

func TestListJobs_Pagination(t *testing.T) {
	c := qt.New(t)
	ctx := c.Context()

	jobManager, client := setupJobsIntegrationTest(c)

	// Insert 5 test jobs
	for range 5 {
		_, err := client.Insert(ctx, successJobArgs{Name: "test"}, nil)
		c.Assert(err, qt.IsNil)
	}

	// Wait for all jobs to complete
	waitForJobs(c, client, 5, defaultTestTimeout)

	// Test first page (3 items)
	resp, err := jobManager.ListJobs(ctx, apiparams.ListJobsRequest{
		Count: 3,
	})
	c.Assert(err, qt.IsNil)
	c.Assert(len(resp.Jobs), qt.Equals, 3)
	c.Assert(resp.NextCursor, qt.Not(qt.Equals), "")

	// Verify no duplicate IDs in first page
	firstPageIDs := make(map[int64]bool)
	for _, job := range resp.Jobs {
		c.Assert(firstPageIDs[job.ID], qt.IsFalse, qt.Commentf("Duplicate job ID %d in first page", job.ID))
		firstPageIDs[job.ID] = true
	}

	// Test second page using cursor (5 remaining items)
	resp2, err := jobManager.ListJobs(ctx, apiparams.ListJobsRequest{
		Count:  3,
		Cursor: resp.NextCursor,
	})
	c.Assert(err, qt.IsNil)
	c.Assert(len(resp2.Jobs), qt.Equals, 2)

	// Verify no duplicate IDs between pages
	for _, job := range resp2.Jobs {
		c.Assert(firstPageIDs[job.ID], qt.IsFalse, qt.Commentf("Job ID %d appears in both pages", job.ID))
	}

	// Verify third page is empty
	if resp2.NextCursor != "" {
		resp3, err := jobManager.ListJobs(ctx, apiparams.ListJobsRequest{
			Count:  10,
			Cursor: resp2.NextCursor,
		})
		c.Assert(err, qt.IsNil)
		c.Assert(len(resp3.Jobs), qt.Equals, 0)
	}
}

func TestListJobs_ErrorState(t *testing.T) {
	c := qt.New(t)
	ctx := c.Context()

	jobManager, client := setupJobsIntegrationTest(c)

	// Insert jobs that will fail
	for range 3 {
		_, err := client.Insert(ctx, failureJobArgs{
			Name: "test-fail",
		}, nil)
		c.Assert(err, qt.IsNil)
	}

	// Insert some successful jobs for comparison
	for range 2 {
		_, err := client.Insert(ctx, successJobArgs{Name: "test-success"}, nil)
		c.Assert(err, qt.IsNil)
	}

	// Wait for all jobs to complete
	waitForJobs(c, client, 5, defaultTestTimeout)

	// Test filtering by failed status
	resp, err := jobManager.ListJobs(ctx, apiparams.ListJobsRequest{
		Statuses: []apiparams.JobStatus{apiparams.StatusFailed},
		Count:    100,
	})
	c.Assert(err, qt.IsNil)
	c.Assert(len(resp.Jobs), qt.Equals, 3, qt.Commentf("Expected 3 failed jobs, got %d", len(resp.Jobs)))

	for _, job := range resp.Jobs {
		c.Assert(job.Status, qt.Equals, apiparams.StatusFailed)
		c.Assert(job.Kind, qt.Equals, "test-failure-job")
	}

	// Test filtering by successful status
	respSuccess, err := jobManager.ListJobs(ctx, apiparams.ListJobsRequest{
		Statuses: []apiparams.JobStatus{apiparams.StatusSuccessful},
		Count:    100,
	})
	c.Assert(err, qt.IsNil)
	c.Assert(len(respSuccess.Jobs), qt.Equals, 2, qt.Commentf("Expected 2 successful jobs, got %d", len(respSuccess.Jobs)))

	for _, job := range respSuccess.Jobs {
		c.Assert(job.Status, qt.Equals, apiparams.StatusSuccessful)
		c.Assert(job.Kind, qt.Equals, "test-success-job")
	}
}

func TestListJobs_FilterByKind(t *testing.T) {
	c := qt.New(t)
	ctx := c.Context()

	jobManager, client := setupJobsIntegrationTest(c)

	// Insert success jobs
	for range 3 {
		_, err := client.Insert(ctx, successJobArgs{Name: "test-success"}, nil)
		c.Assert(err, qt.IsNil)
	}

	// Insert failure jobs
	for range 2 {
		_, err := client.Insert(ctx, failureJobArgs{Name: "test-failure"}, nil)
		c.Assert(err, qt.IsNil)
	}

	// Wait for all jobs to complete
	waitForJobs(c, client, 5, defaultTestTimeout)

	// Test with empty kinds filter - should return all jobs
	respAll, err := jobManager.ListJobs(ctx, apiparams.ListJobsRequest{
		Kinds: []string{},
		Count: 100,
	})
	c.Assert(err, qt.IsNil)
	c.Assert(len(respAll.Jobs) >= 5, qt.IsTrue, qt.Commentf("Expected at least 5 jobs, got %d", len(respAll.Jobs)))

	// Count jobs by kind
	successCount := 0
	failureCount := 0
	for _, job := range respAll.Jobs {
		switch job.Kind {
		case "test-success-job":
			successCount++
		case "test-failure-job":
			failureCount++
		}
	}
	c.Assert(successCount, qt.Equals, 3, qt.Commentf("Expected 3 success jobs, got %d", successCount))
	c.Assert(failureCount, qt.Equals, 2, qt.Commentf("Expected 2 failure jobs, got %d", failureCount))
}

func TestGetUpgradeToStatusForModel_UsesOutputInfo(t *testing.T) {
	c := qt.New(t)
	ctx := c.Context()

	jobManager, client := setupJobsIntegrationTest(c)
	modelUUID := "93608db4-f1cb-4da5-9926-8233981aef0a"
	info := "Upgrading model to version 4.0.0"

	metadata, err := json.Marshal(rivertypes.JobModelUUIDMetadata{ModelUUID: modelUUID})
	c.Assert(err, qt.IsNil)
	rootRes, err := client.Insert(ctx, rivertypes.UpgradeToArgs{
		ModelUUID:            modelUUID,
		Username:             "alice@canonical.com",
		TargetControllerName: "target-controller",
	}, &river.InsertOpts{Metadata: metadata, Queue: "inactive"})
	c.Assert(err, qt.IsNil)

	_, err = client.JobUpdate(ctx, rootRes.Job.ID, &river.JobUpdateParams{
		Output: rivertypes.UpgradeToOutput{Info: info},
	})
	c.Assert(err, qt.IsNil)

	status, err := jobManager.GetUpgradeToStatusForModel(ctx, modelUUID)
	c.Assert(err, qt.IsNil)
	c.Assert(status, qt.IsNotNil)
	c.Assert(status.Detail.State, qt.Equals, string(rootRes.Job.State))
	c.Assert(status.Detail.Attempt, qt.Equals, rootRes.Job.Attempt)
	c.Assert(status.Detail.MaxAttempts, qt.Equals, rootRes.Job.MaxAttempts)
	c.Assert(status.Info, qt.Equals, info)
}

func TestGetUpgradeToStatusForModel_InvalidOutput(t *testing.T) {
	c := qt.New(t)
	ctx := c.Context()

	jobManager, client := setupJobsIntegrationTest(c)
	modelUUID := "93608db4-f1cb-4da5-9926-8233981aef0a"

	metadata, err := json.Marshal(rivertypes.JobModelUUIDMetadata{ModelUUID: modelUUID})
	c.Assert(err, qt.IsNil)
	rootRes, err := client.Insert(ctx, rivertypes.UpgradeToArgs{
		ModelUUID:            modelUUID,
		Username:             "alice@canonical.com",
		TargetControllerName: "target-controller",
	}, &river.InsertOpts{Metadata: metadata, Queue: "inactive"})
	c.Assert(err, qt.IsNil)

	_, err = client.JobUpdate(ctx, rootRes.Job.ID, &river.JobUpdateParams{
		Output: map[string]any{
			"info": map[string]any{"detail": "wrong-shape"},
		},
	})
	c.Assert(err, qt.IsNil)

	status, err := jobManager.GetUpgradeToStatusForModel(ctx, modelUUID)
	c.Assert(err, qt.ErrorMatches, "failed to decode upgrade-to output: .*")
	c.Assert(status, qt.IsNil)
}

func TestGetUpgradeToStatusForModel_UsesLatestFinalizedRoot(t *testing.T) {
	c := qt.New(t)
	ctx := c.Context()

	jobManager, client := setupJobsIntegrationTest(c)
	modelUUID := "93608db4-f1cb-4da5-9926-8233981aef0a"
	metadata, err := json.Marshal(rivertypes.JobModelUUIDMetadata{ModelUUID: modelUUID})
	c.Assert(err, qt.IsNil)

	_, err = client.Insert(ctx, rivertypes.UpgradeToArgs{
		ModelUUID:            modelUUID,
		Username:             "discard-first",
		TargetControllerName: "target-controller",
	}, &river.InsertOpts{Metadata: metadata, MaxAttempts: 1})
	c.Assert(err, qt.IsNil)
	waitForJobs(c, client, 1, defaultTestTimeout)

	_, err = client.Insert(ctx, rivertypes.UpgradeToArgs{
		ModelUUID:            modelUUID,
		Username:             "discard-second",
		TargetControllerName: "target-controller",
	}, &river.InsertOpts{Metadata: metadata, MaxAttempts: 1})
	c.Assert(err, qt.IsNil)
	waitForJobs(c, client, 1, defaultTestTimeout)

	status, err := jobManager.GetUpgradeToStatusForModel(ctx, modelUUID)
	c.Assert(err, qt.IsNil)
	c.Assert(status, qt.IsNotNil)
	c.Assert(status.Detail.State, qt.Equals, string(rivertype.JobStateDiscarded))
	c.Assert(status.Detail.Errors, qt.HasLen, 1)
	c.Assert(status.Detail.Errors[0].Error, qt.Equals, "discarded upgrade root for discard-second")

	_, err = client.Insert(ctx, rivertypes.UpgradeToArgs{
		ModelUUID:            modelUUID,
		Username:             "complete-third",
		TargetControllerName: "target-controller",
	}, &river.InsertOpts{Metadata: metadata, MaxAttempts: 1})
	c.Assert(err, qt.IsNil)
	waitForJobs(c, client, 1, defaultTestTimeout)

	status, err = jobManager.GetUpgradeToStatusForModel(ctx, modelUUID)
	c.Assert(err, qt.IsNil)
	c.Assert(status, qt.IsNotNil)
	c.Assert(status.Detail.State, qt.Equals, string(rivertype.JobStateCompleted))
	c.Assert(status.Detail.Errors, qt.HasLen, 0)
}

func TestListUpgradeToJobsForModels_MultipleModels(t *testing.T) {
	c := qt.New(t)
	ctx := c.Context()

	jobManager, client := setupJobsIntegrationTest(c)

	requestedModelUUID1 := "93608db4-f1cb-4da5-9926-8233981aef0a"
	requestedModelUUID2 := "93608db4-f1cb-4da5-9926-8233981aef0b"
	nonRequestedModelUUID := "93608db4-f1cb-4da5-9926-8233981aef0c"

	for _, testCase := range []struct {
		modelUUID string
		username  string
		queue     string
	}{
		{modelUUID: requestedModelUUID1, username: "alice@canonical.com", queue: "inactive"},
		{modelUUID: requestedModelUUID2, username: "bob@canonical.com", queue: "inactive"},
		{modelUUID: nonRequestedModelUUID, username: "carol@canonical.com", queue: "inactive"},
	} {
		metadata, err := json.Marshal(rivertypes.JobModelUUIDMetadata{ModelUUID: testCase.modelUUID})
		c.Assert(err, qt.IsNil)

		_, err = client.Insert(ctx, rivertypes.UpgradeToArgs{
			ModelUUID:            testCase.modelUUID,
			Username:             testCase.username,
			TargetControllerName: "target-controller",
		}, &river.InsertOpts{Metadata: metadata, Queue: testCase.queue, MaxAttempts: 1})
		c.Assert(err, qt.IsNil)
	}

	jobsByModelUUID, err := jobManager.ListUpgradeToJobsForModels(ctx, []string{
		requestedModelUUID1,
		requestedModelUUID2,
	})
	c.Assert(err, qt.IsNil)
	c.Assert(jobsByModelUUID, qt.DeepEquals, map[string]string{
		requestedModelUUID1: UpgradeToModelStatusProgress,
		requestedModelUUID2: UpgradeToModelStatusProgress,
	})
}

func TestListUpgradeToJobsForModels_CompletedModel(t *testing.T) {
	c := qt.New(t)
	ctx := c.Context()

	jobManager, client := setupJobsIntegrationTest(c)
	modelUUID := "93608db4-f1cb-4da5-9926-8233981aef0a"

	metadata, err := json.Marshal(rivertypes.JobModelUUIDMetadata{ModelUUID: modelUUID})
	c.Assert(err, qt.IsNil)

	_, err = client.Insert(ctx, rivertypes.UpgradeToArgs{
		ModelUUID:            modelUUID,
		Username:             "complete-model",
		TargetControllerName: "target-controller",
	}, &river.InsertOpts{Metadata: metadata, MaxAttempts: 1})
	c.Assert(err, qt.IsNil)

	waitForJobs(c, client, 1, defaultTestTimeout)

	jobsByModelUUID, err := jobManager.ListUpgradeToJobsForModels(ctx, []string{modelUUID})
	c.Assert(err, qt.IsNil)
	c.Assert(jobsByModelUUID, qt.DeepEquals, map[string]string{
		modelUUID: UpgradeToModelStatusCompleted,
	})
}
