// Copyright 2026 Canonical.

package river

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"testing"

	qt "github.com/frankban/quicktest"
	"github.com/juju/version/v2"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverdatabasesql"
	"github.com/riverqueue/river/rivertype"
	gomock "go.uber.org/mock/gomock"

	"github.com/canonical/jimm/v3/internal/db"
	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/openfga"
)

func TestUpgradeToWorker_Success(t *testing.T) {
	c := qt.New(t)
	ctx := c.Context()

	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	database := setupTestDB(c)
	sqlDB, err := database.SqlDB()
	c.Assert(err, qt.IsNil)

	upgradeManager := NewMockUpgradeManager(ctrl)

	riverClient, username := setupWorkers(
		c,
		ctx,
		setupWorkerParams{
			database:          database,
			upgradeManager:    upgradeManager,
			sqlDB:             sqlDB,
			migrateRetryCount: 1,
			upgradeRetryCount: 1,
			awaitFunc:         waitForJobToFinalise,
		},
	)

	upgradeManager.EXPECT().
		MigrateModel(gomock.Any(), gomock.Any(), "model-uuid", "target-controller").
		Return(nil)
	upgradeManager.EXPECT().
		UpgradeModel(gomock.Any(), "model-uuid", version.MustParse("2.0.0")).
		Return(nil)

	sub, cancel := riverClient.Subscribe(river.EventKindJobCompleted)
	c.Cleanup(cancel)

	insRes, err := riverClient.Insert(ctx, UpgradeToArgs{
		ModelUUID:            "model-uuid",
		TargetVersion:        version.MustParse("2.0.0"),
		Username:             username,
		TargetControllerName: "target-controller",
	}, nil)
	c.Assert(err, qt.IsNil)

	row := waitForFinalisedJob(c, ctx, sub, insRes)
	c.Assert(row.State, qt.Equals, rivertype.JobStateCompleted)
	c.Assert(row.Errors, qt.HasLen, 0)
}

func TestUpgradeToWorker_SuccessCanBeUpgradedToAgain(t *testing.T) {
	c := qt.New(t)
	ctx := c.Context()

	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	database := setupTestDB(c)
	sqlDB, err := database.SqlDB()
	c.Assert(err, qt.IsNil)

	upgradeManager := NewMockUpgradeManager(ctrl)

	riverClient, username := setupWorkers(
		c,
		ctx,
		setupWorkerParams{
			database:          database,
			upgradeManager:    upgradeManager,
			sqlDB:             sqlDB,
			migrateRetryCount: 1,
			upgradeRetryCount: 1,
			awaitFunc:         waitForJobToFinalise,
		},
	)

	upgradeManager.EXPECT().
		MigrateModel(gomock.Any(), gomock.Any(), "model-uuid", "target-controller").
		Return(nil)
	upgradeManager.EXPECT().
		UpgradeModel(gomock.Any(), "model-uuid", version.MustParse("2.0.0")).
		Return(nil)

	sub, cancel := riverClient.Subscribe(river.EventKindJobCompleted)
	c.Cleanup(cancel)

	insRes, err := riverClient.Insert(ctx, UpgradeToArgs{
		ModelUUID:            "model-uuid",
		TargetVersion:        version.MustParse("2.0.0"),
		Username:             username,
		TargetControllerName: "target-controller",
	}, nil)
	c.Assert(err, qt.IsNil)

	row := waitForFinalisedJob(c, ctx, sub, insRes)
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

	insRes, err = riverClient.Insert(ctx, UpgradeToArgs{
		ModelUUID:            "model-uuid",
		TargetVersion:        version.MustParse("3.0.0"),
		Username:             username,
		TargetControllerName: "target-controller2",
	}, nil)
	c.Assert(err, qt.IsNil)

	row = waitForFinalisedJob(c, ctx, sub, insRes)
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

	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	database := setupTestDB(c)
	sqlDB, err := database.SqlDB()
	c.Assert(err, qt.IsNil)

	upgradeManager := NewMockUpgradeManager(ctrl)

	// Retry a few times to ensure retries work as expected and surface the LAST error.
	riverClient, username := setupWorkers(
		c,
		ctx,
		setupWorkerParams{
			database:          database,
			upgradeManager:    upgradeManager,
			sqlDB:             sqlDB,
			migrateRetryCount: 3,
			upgradeRetryCount: 1,
			awaitFunc:         waitForJobToFinalise,
		},
	)

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

	insRes, err := riverClient.Insert(ctx, UpgradeToArgs{
		ModelUUID:            "model-uuid",
		TargetVersion:        version.MustParse("2.0.0"),
		Username:             username,
		TargetControllerName: "target-controller",
	}, &river.InsertOpts{MaxAttempts: 1})
	c.Assert(err, qt.IsNil)

	row := waitForFinalisedJob(c, ctx, sub, insRes)
	c.Assert(row.State, qt.Equals, rivertype.JobStateDiscarded)
	// Ensure we capture the last error only from the migrate job, and that it is surfaced to the upgrade to job.
	upgradeToJobFinalError := row.Errors[len(row.Errors)-1].Error
	c.Assert(upgradeToJobFinalError, qt.Equals, "unexpected-error-3")
}

func TestUpgradeToWorker_UpgradeFails(t *testing.T) {
	c := qt.New(t)
	ctx := c.Context()

	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	database := setupTestDB(c)
	sqlDB, err := database.SqlDB()
	c.Assert(err, qt.IsNil)

	upgradeManager := NewMockUpgradeManager(ctrl)

	// Retry a few times to ensure retries work as expected and surface the LAST error.
	riverClient, username := setupWorkers(
		c,
		ctx,
		setupWorkerParams{
			database:          database,
			upgradeManager:    upgradeManager,
			sqlDB:             sqlDB,
			migrateRetryCount: 1,
			upgradeRetryCount: 3,
			awaitFunc:         waitForJobToFinalise,
		},
	)

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

	insRes, err := riverClient.Insert(ctx, UpgradeToArgs{
		ModelUUID:            "model-uuid",
		TargetVersion:        version.MustParse("2.0.0"),
		Username:             username,
		TargetControllerName: "target-controller",
	}, &river.InsertOpts{MaxAttempts: 1})
	c.Assert(err, qt.IsNil)

	row := waitForFinalisedJob(c, ctx, sub, insRes)
	c.Assert(row.State, qt.Equals, rivertype.JobStateDiscarded)
	// Ensure we capture the last error only from the migrate job, and that it is surfaced to the upgrade to job.
	upgradeToJobFinalError := row.Errors[len(row.Errors)-1].Error
	c.Assert(upgradeToJobFinalError, qt.Equals, "unexpected-error-3")
}

// This test is particularly valuable because it ensures we're checking the jobs finalised state AND event kind.
func TestUpgradeToWorker_SuccessAfterTransientFailures(t *testing.T) {
	c := qt.New(t)
	ctx := c.Context()

	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	database := setupTestDB(c)
	sqlDB, err := database.SqlDB()
	c.Assert(err, qt.IsNil)

	upgradeManager := NewMockUpgradeManager(ctrl)

	// Allow each child to fail once and then succeed.
	riverClient, username := setupWorkers(
		c,
		ctx,
		setupWorkerParams{
			database:          database,
			upgradeManager:    upgradeManager,
			sqlDB:             sqlDB,
			migrateRetryCount: 2,
			upgradeRetryCount: 2,
			awaitFunc:         waitForJobToFinalise,
		},
	)

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

	insRes, err := riverClient.Insert(ctx, UpgradeToArgs{
		ModelUUID:            "model-uuid",
		TargetVersion:        version.MustParse("2.0.0"),
		Username:             username,
		TargetControllerName: "target-controller",
	}, nil)
	c.Assert(err, qt.IsNil)

	row := waitForFinalisedJob(c, ctx, sub, insRes)
	c.Assert(row.State, qt.Equals, rivertype.JobStateCompleted)
	c.Assert(row.Errors, qt.HasLen, 0)
}

func TestUpgradeToWorker_EnsureCancellingSupervisorCancelsSpawnedMigrateJob(t *testing.T) {
	c := qt.New(t)
	ctx := c.Context()

	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	database := setupTestDB(c)
	sqlDB, err := database.SqlDB()
	c.Assert(err, qt.IsNil)

	upgradeManager := NewMockUpgradeManager(ctrl)

	supervisingJobId := int64(1)

	riverClient, username := setupWorkers(
		c,
		ctx,
		setupWorkerParams{
			database:          database,
			upgradeManager:    upgradeManager,
			sqlDB:             sqlDB,
			migrateRetryCount: 3,
			upgradeRetryCount: 1,
			awaitFunc:         waitForJobToFinalise,
		},
	)

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

	_, err = riverClient.Insert(ctx, UpgradeToArgs{
		ModelUUID:            "model-uuid",
		TargetVersion:        version.MustParse("2.0.0"),
		Username:             username,
		TargetControllerName: "target-controller",
	}, nil)
	c.Assert(err, qt.IsNil)

	supervisingJobFailureUpdate := waitForSupervisingJob(c, ctx, sub, supervisingJobId)

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

	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	database := setupTestDB(c)
	sqlDB, err := database.SqlDB()
	c.Assert(err, qt.IsNil)

	upgradeManager := NewMockUpgradeManager(ctrl)

	migrateWaitToComplete := make(chan struct{})

	// It'll crash once after the migrate job is inserted.
	// We'll check the attempts of the migrate job after success of the supervisor.
	crash := true
	var once sync.Once

	riverClient, username := setupWorkers(
		c,
		ctx,
		setupWorkerParams{
			database:          database,
			upgradeManager:    upgradeManager,
			sqlDB:             sqlDB,
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

	insRes, err := riverClient.Insert(ctx, UpgradeToArgs{
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
	supervisorRow := waitForFinalisedJob(c, ctx, sub, insRes)
	c.Assert(supervisorRow.State, qt.Equals, rivertype.JobStateCompleted)
	c.Assert(supervisorRow.Attempt, qt.Equals, 2)

	// Check the migrate was inserted only once.
	migrateParams := river.NewJobListParams().Kinds(migrationWorkerArgs{}.Kind()).First(2)
	migrateListRes, err := riverClient.JobList(ctx, migrateParams)
	c.Assert(err, qt.IsNil)
	c.Assert(migrateListRes.Jobs, qt.HasLen, 1)
}

type setupWorkerParams struct {
	database          *db.Database
	upgradeManager    UpgradeManager
	sqlDB             *sql.DB
	migrateRetryCount int
	upgradeRetryCount int
	awaitFunc         awaitCompletionFunc
}

func setupWorkers(
	c *qt.C,
	ctx context.Context,
	p setupWorkerParams,
) (*river.Client[*sql.Tx], string) {
	// Prepare identity needed by migrationWorker.
	u, err := dbmodel.NewIdentity("ash@catchum.com")
	c.Assert(err, qt.IsNil)
	err = p.database.GetIdentity(c.Context(), u)
	c.Assert(err, qt.IsNil)

	openfgaClient := &openfga.OFGAClient{}
	migrationW, err := newMigrationWorker(openfgaClient, p.database, p.upgradeManager)
	c.Assert(err, qt.IsNil)
	upgradeW, err := newUpgradeWorker(p.upgradeManager)
	c.Assert(err, qt.IsNil)
	upgradeToW := newUpgradeToWorker(p.migrateRetryCount, p.upgradeRetryCount, p.awaitFunc)

	workers := river.NewWorkers()
	c.Assert(river.AddWorkerSafely(workers, migrationW), qt.IsNil)
	c.Assert(river.AddWorkerSafely(workers, upgradeW), qt.IsNil)
	c.Assert(river.AddWorkerSafely(workers, upgradeToW), qt.IsNil)

	riverClient, err := river.NewClient(riverdatabasesql.New(p.sqlDB), &river.Config{
		TestOnly: true,
		Queues: map[string]river.QueueConfig{
			river.QueueDefault: {MaxWorkers: 5},
		},
		Workers: workers,
	})
	c.Assert(err, qt.IsNil)

	c.Assert(riverClient.Start(ctx), qt.IsNil)
	c.Cleanup(func() {
		err := riverClient.Stop(context.Background())
		c.Check(err, qt.IsNil)
	})

	return riverClient, u.Name
}

func waitForFinalisedJob(c *qt.C, ctx context.Context, sub <-chan *river.Event, insRes *rivertype.JobInsertResult) *rivertype.JobRow {
loop:
	for {
		select {
		case event := <-sub:
			c.Logf("received job failed event for job ID %d", event.Job.ID)
			if event.Job.ID != insRes.Job.ID {
				continue loop
			}
			if event.Job.FinalizedAt != nil {
				return event.Job
			}
		case <-ctx.Done():
			c.Fatal("timed out waiting for job failed event")
		}
	}
}

func waitForSupervisingJob(c *qt.C, ctx context.Context, sub <-chan *river.Event, supervisingJobId int64) *rivertype.JobRow {
	for {
		select {
		case event := <-sub:
			if event.Job.ID == supervisingJobId {
				return event.Job
			}
		case <-ctx.Done():
			c.Fatal("timed out waiting for job failed event")
		}
	}
}
