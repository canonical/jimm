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
