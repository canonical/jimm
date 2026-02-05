// Copyright 2026 Canonical.

package river

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	qt "github.com/frankban/quicktest"
	"github.com/juju/version/v2"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
	gomock "go.uber.org/mock/gomock"

	"github.com/canonical/jimm/v3/internal/openfga"
	"github.com/canonical/jimm/v3/internal/rivertypes"
)

func TestUpgradeToWorker_Success(t *testing.T) {
	c := qt.New(t)
	ctx := c.Context()

	testDeps := setupIntegrationTest(
		c,
		setupWorkerParams{
			migrateRetryCount: 1,
			upgradeRetryCount: 1,
			awaitFunc:         waitForJobToFinalise,
		},
	)

	riverClient := testDeps.riverClient
	username := testDeps.identity
	upgradeManager := testDeps.mockUpgradeManager

	upgradeManager.EXPECT().
		MigrateModel(gomock.Any(), gomock.Any(), "model-uuid", "target-controller").
		Return(nil)
	upgradeManager.EXPECT().
		UpgradeModel(gomock.Any(), "model-uuid", version.MustParse("2.0.0")).
		Return(nil)

	sub, cancel := riverClient.Subscribe(river.EventKindJobCompleted)
	c.Cleanup(cancel)

	insRes, err := riverClient.Insert(ctx, rivertypes.UpgradeToArgs{
		ModelUUID:            "model-uuid",
		TargetVersion:        version.MustParse("2.0.0"),
		Username:             username,
		TargetControllerName: "target-controller",
	}, nil)
	c.Assert(err, qt.IsNil)

	row := waitForFinalisedJob(c, ctx, sub, insRes.Job.ID)
	c.Assert(row.State, qt.Equals, rivertype.JobStateCompleted)
	c.Assert(row.Errors, qt.HasLen, 0)
}

func TestUpgradeToWorker_SuccessCanBeUpgradedToAgain(t *testing.T) {
	c := qt.New(t)
	ctx := c.Context()

	testDeps := setupIntegrationTest(
		c,
		setupWorkerParams{
			migrateRetryCount: 1,
			upgradeRetryCount: 1,
			awaitFunc:         waitForJobToFinalise,
		},
	)

	riverClient := testDeps.riverClient
	username := testDeps.identity
	upgradeManager := testDeps.mockUpgradeManager

	upgradeManager.EXPECT().
		MigrateModel(gomock.Any(), gomock.Any(), "model-uuid", "target-controller").
		Return(nil)
	upgradeManager.EXPECT().
		UpgradeModel(gomock.Any(), "model-uuid", version.MustParse("2.0.0")).
		Return(nil)

	sub, cancel := riverClient.Subscribe(river.EventKindJobCompleted)
	c.Cleanup(cancel)

	insRes, err := riverClient.Insert(ctx, rivertypes.UpgradeToArgs{
		ModelUUID:            "model-uuid",
		TargetVersion:        version.MustParse("2.0.0"),
		Username:             username,
		TargetControllerName: "target-controller",
	}, nil)
	c.Assert(err, qt.IsNil)

	row := waitForFinalisedJob(c, ctx, sub, insRes.Job.ID)
	c.Assert(row.ID, qt.Equals, int64(1))
	c.Assert(row.State, qt.Equals, rivertype.JobStateCompleted)
	c.Assert(row.Errors, qt.HasLen, 0)

	// Now we'll upgrade again to a new controller and new version, but the same model.
	upgradeManager.EXPECT().
		MigrateModel(gomock.Any(), gomock.Any(), "model-uuid", "target-controller2").
		Return(nil)
	upgradeManager.EXPECT().
		UpgradeModel(gomock.Any(), "model-uuid", version.MustParse("3.0.0")).
		Return(nil)

	insRes, err = riverClient.Insert(ctx, rivertypes.UpgradeToArgs{
		ModelUUID:            "model-uuid",
		TargetVersion:        version.MustParse("3.0.0"),
		Username:             username,
		TargetControllerName: "target-controller2",
	}, nil)
	c.Assert(err, qt.IsNil)

	row = waitForFinalisedJob(c, ctx, sub, insRes.Job.ID)
	c.Assert(row.ID, qt.Equals, int64(4))
	c.Assert(row.State, qt.Equals, rivertype.JobStateCompleted)
	c.Assert(row.Errors, qt.HasLen, 0)

	// We can see two distinct jobs now because the first one entered a completed state.
	c.Assert(insRes.UniqueSkippedAsDuplicate, qt.IsFalse)

	// Finally, verify further that there's two migrate and two upgrade jobs now, so we know
	// it re-created them for the second run of UpgradeTo.
	listRes, err := riverClient.JobList(ctx, river.NewJobListParams().Kinds(migrationWorkerArgs{}.Kind()).First(10))
	c.Assert(err, qt.IsNil)
	c.Assert(listRes.Jobs[0].State, qt.Equals, rivertype.JobStateCompleted)
	c.Assert(listRes.Jobs, qt.HasLen, 2)

	listRes, err = riverClient.JobList(ctx, river.NewJobListParams().Kinds(upgradeWorkerArgs{}.Kind()).First(10))
	c.Assert(err, qt.IsNil)
	c.Assert(listRes.Jobs[0].State, qt.Equals, rivertype.JobStateCompleted)
	c.Assert(listRes.Jobs, qt.HasLen, 2)
}

func TestUpgradeToWorker_MigrationFails(t *testing.T) {
	c := qt.New(t)
	ctx := c.Context()

	testDeps := setupIntegrationTest(
		c,
		setupWorkerParams{
			migrateRetryCount: 3,
			upgradeRetryCount: 1,
			awaitFunc:         waitForJobToFinalise,
		},
	)

	riverClient := testDeps.riverClient
	username := testDeps.identity
	upgradeManager := testDeps.mockUpgradeManager

	attempt := 0
	upgradeManager.EXPECT().
		MigrateModel(gomock.Any(), gomock.Any(), "model-uuid", "target-controller").
		DoAndReturn(func(context.Context, *openfga.User, string, string) error {
			attempt++
			return fmt.Errorf("unexpected-error-%d", attempt)
		}).
		MinTimes(3)

	sub, cancel := riverClient.Subscribe(river.EventKindJobFailed)
	c.Cleanup(cancel)

	insRes, err := riverClient.Insert(ctx, rivertypes.UpgradeToArgs{
		ModelUUID:            "model-uuid",
		TargetVersion:        version.MustParse("2.0.0"),
		Username:             username,
		TargetControllerName: "target-controller",
	}, &river.InsertOpts{MaxAttempts: 1})
	c.Assert(err, qt.IsNil)

	row := waitForFinalisedJob(c, ctx, sub, insRes.Job.ID)
	c.Assert(row.State, qt.Equals, rivertype.JobStateDiscarded)
	// Ensure we capture the last error only from the migrate job, and that it is surfaced to the upgrade to job.
	upgradeToJobFinalError := row.Errors[len(row.Errors)-1].Error
	c.Assert(upgradeToJobFinalError, qt.Equals, "unexpected-error-3")
}

func TestUpgradeToWorker_UpgradeFails(t *testing.T) {
	c := qt.New(t)
	ctx := c.Context()

	testDeps := setupIntegrationTest(
		c,
		setupWorkerParams{
			migrateRetryCount: 1,
			upgradeRetryCount: 3,
			awaitFunc:         waitForJobToFinalise,
		},
	)

	riverClient := testDeps.riverClient
	username := testDeps.identity
	upgradeManager := testDeps.mockUpgradeManager

	upgradeManager.EXPECT().
		MigrateModel(gomock.Any(), gomock.Any(), "model-uuid", "target-controller").
		Return(nil)

	attempt := 0
	upgradeManager.EXPECT().
		UpgradeModel(gomock.Any(), "model-uuid", version.MustParse("2.0.0")).
		DoAndReturn(func(context.Context, string, version.Number) error {
			attempt++
			return fmt.Errorf("unexpected-error-%d", attempt)
		}).
		MinTimes(3)

	sub, cancel := riverClient.Subscribe(river.EventKindJobFailed)
	c.Cleanup(cancel)

	insRes, err := riverClient.Insert(ctx, rivertypes.UpgradeToArgs{
		ModelUUID:            "model-uuid",
		TargetVersion:        version.MustParse("2.0.0"),
		Username:             username,
		TargetControllerName: "target-controller",
	}, &river.InsertOpts{MaxAttempts: 1})
	c.Assert(err, qt.IsNil)

	row := waitForFinalisedJob(c, ctx, sub, insRes.Job.ID)
	c.Assert(row.State, qt.Equals, rivertype.JobStateDiscarded)
	// Ensure we capture the last error only from the migrate job, and that it is surfaced to the upgrade to job.
	upgradeToJobFinalError := row.Errors[len(row.Errors)-1].Error
	c.Assert(upgradeToJobFinalError, qt.Equals, "unexpected-error-3")
}

// This test is particularly valuable because it ensures we're checking the jobs finalised state AND event kind.
func TestUpgradeToWorker_SuccessAfterTransientFailures(t *testing.T) {
	c := qt.New(t)
	ctx := c.Context()

	testDeps := setupIntegrationTest(
		c,
		setupWorkerParams{
			migrateRetryCount: 2,
			upgradeRetryCount: 2,
			awaitFunc:         waitForJobToFinalise,
		},
	)

	riverClient := testDeps.riverClient
	username := testDeps.identity
	upgradeManager := testDeps.mockUpgradeManager

	migAttempt := 0
	upgradeManager.EXPECT().
		MigrateModel(gomock.Any(), gomock.Any(), "model-uuid", "target-controller").
		DoAndReturn(func(context.Context, *openfga.User, string, string) error {
			migAttempt++
			if migAttempt == 1 {
				return errors.New("migration-transient")
			}
			// Any subsequent invocation should succeed so the job can finalize as completed.
			return nil
		}).
		MinTimes(2)

	upAttempt := 0
	upgradeManager.EXPECT().
		UpgradeModel(gomock.Any(), "model-uuid", version.MustParse("2.0.0")).
		DoAndReturn(func(context.Context, string, version.Number) error {
			upAttempt++
			if upAttempt == 1 {
				return errors.New("upgrade-transient")
			}
			return nil
		}).
		Times(2)

	sub, cancel := riverClient.Subscribe(river.EventKindJobCompleted)
	c.Cleanup(cancel)

	insRes, err := riverClient.Insert(ctx, rivertypes.UpgradeToArgs{
		ModelUUID:            "model-uuid",
		TargetVersion:        version.MustParse("2.0.0"),
		Username:             username,
		TargetControllerName: "target-controller",
	}, nil)
	c.Assert(err, qt.IsNil)

	row := waitForFinalisedJob(c, ctx, sub, insRes.Job.ID)
	c.Assert(row.State, qt.Equals, rivertype.JobStateCompleted)
	c.Assert(row.Errors, qt.HasLen, 0)
}

func TestUpgradeToWorker_EnsureCancellingSupervisorCancelsSpawnedMigrateJob(t *testing.T) {
	c := qt.New(t)
	ctx := c.Context()

	supervisingJobId := int64(1)

	testDeps := setupIntegrationTest(
		c,
		setupWorkerParams{
			migrateRetryCount: 3,
			upgradeRetryCount: 1,
			awaitFunc:         waitForJobToFinalise,
		},
	)

	riverClient := testDeps.riverClient
	username := testDeps.identity
	upgradeManager := testDeps.mockUpgradeManager

	attempt := 0
	upgradeManager.EXPECT().
		MigrateModel(gomock.Any(), gomock.Any(), "model-uuid", "target-controller").
		DoAndReturn(func(context.Context, *openfga.User, string, string) error {
			attempt++
			if attempt == 1 {
				_, err := riverClient.JobCancel(ctx, supervisingJobId)
				if err != nil {
					return err
				}
			}
			// We'll be returning nil on restart, and expect the job to complete successfully.
			return nil
		})

	upgradeManager.EXPECT().
		UpgradeModel(gomock.Any(), "model-uuid", version.MustParse("2.0.0")).
		Return(nil)

	sub, cancel := riverClient.Subscribe(river.EventKindJobFailed, river.EventKindJobCompleted)
	c.Cleanup(cancel)

	_, err := riverClient.Insert(ctx, rivertypes.UpgradeToArgs{
		ModelUUID:            "model-uuid",
		TargetVersion:        version.MustParse("2.0.0"),
		Username:             username,
		TargetControllerName: "target-controller",
	}, nil)
	c.Assert(err, qt.IsNil)

	supervisingJobFailureUpdate := waitForFinalisedJob(c, ctx, sub, supervisingJobId)

	// At this point, our job is cancelled and we'll see it as "completed".
	c.Assert(supervisingJobFailureUpdate.State, qt.Equals, rivertype.JobStateCompleted)

	params := river.NewJobListParams().Kinds(migrationWorkerArgs{}.Kind()).First(10)
	listRes, err := riverClient.JobList(ctx, params)
	c.Assert(err, qt.IsNil)
	c.Assert(listRes.Jobs, qt.HasLen, 1)
	// Check the nested migrate job has finalised successfully because the context
	// was cancelled for the root job.
	c.Assert(listRes.Jobs[0].FinalizedAt, qt.IsNotNil)
}

// The aim of this test is to ensure that only 1 migrate job is inserted, even after a crash.
func TestUpgradeToWorker_SupervisorHandlesCrashMidway(t *testing.T) {
	c := qt.New(t)
	ctx := c.Context()

	migrateWaitToComplete := make(chan struct{})

	// It'll crash once after the migrate job is inserted.
	// We'll check the attempts of the migrate job after success of the supervisor.
	crash := true
	var once sync.Once

	testDeps := setupIntegrationTest(
		c,
		setupWorkerParams{
			migrateRetryCount: 1,
			upgradeRetryCount: 1,
			awaitFunc: func(ctx context.Context, result *rivertype.JobInsertResult, eventCh <-chan *river.Event) error {
				if crash {
					crash = false
					return errors.New("simulated crash")
				}

				once.Do(func() { close(migrateWaitToComplete) })

				return waitForJobToFinalise(ctx, result, eventCh)
			},
		},
	)
	riverClient := testDeps.riverClient
	username := testDeps.identity
	upgradeManager := testDeps.mockUpgradeManager

	upgradeManager.EXPECT().
		MigrateModel(gomock.Any(), gomock.Any(), "model-uuid", "target-controller").
		DoAndReturn(func(context.Context, *openfga.User, string, string) error {
			// We have to prevent the migrate worker from finalising
			// so we can see that upon restart of the supervisor, it isn't
			// inserting a duplicate and is as such crash resillient.
			//
			// Wait here until the supervisor restarts to send the signal to complete.
			<-migrateWaitToComplete
			return nil
		})

	upgradeManager.EXPECT().
		UpgradeModel(gomock.Any(), "model-uuid", version.MustParse("2.0.0")).
		Return(nil)

	sub, cancel := riverClient.Subscribe(river.EventKindJobCompleted)
	c.Cleanup(cancel)

	insRes, err := riverClient.Insert(ctx, rivertypes.UpgradeToArgs{
		ModelUUID:            "model-uuid",
		TargetVersion:        version.MustParse("2.0.0"),
		Username:             username,
		TargetControllerName: "target-controller",
	}, nil)
	c.Assert(err, qt.IsNil)

	// The flow of what is happening here is:
	// 1. Supervisor starts
	// 2. Inserts migrate job which is blocked by channel.
	// 3. Supervisor panics first time waiting on the migrate.
	// 4. Supervisor restarts, and attempts to insert migrate job again, but it's a duplicate.
	// 5. Waits for the migrate to finalise, but this time, unblocks the migrate job just before waiting.
	// 6. 2nd try of supervisor finally completes.
	// And we expect to see the supervisor attempted twice, but migrate once.
	supervisorRow := waitForFinalisedJob(c, ctx, sub, insRes.Job.ID)
	c.Assert(supervisorRow.State, qt.Equals, rivertype.JobStateCompleted)
	c.Assert(supervisorRow.Attempt, qt.Equals, 2)

	// Check the migrate was inserted only once.
	migrateParams := river.NewJobListParams().Kinds(migrationWorkerArgs{}.Kind()).First(2)
	migrateListRes, err := riverClient.JobList(ctx, migrateParams)
	c.Assert(err, qt.IsNil)
	c.Assert(migrateListRes.Jobs, qt.HasLen, 1)
}
