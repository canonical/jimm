package river

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"testing"

	"github.com/canonical/jimm/v3/internal/db"
	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/openfga"
	qt "github.com/frankban/quicktest"
	"github.com/juju/version/v2"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverdatabasesql"
	"github.com/riverqueue/river/rivertype"
	gomock "go.uber.org/mock/gomock"
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

	riverClient, username := setupWorkers(c, ctx, database, upgradeManager, sqlDB, 1, 1)

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
	}, &river.InsertOpts{MaxAttempts: 1})
	c.Assert(err, qt.IsNil)

	row := waitForFinalisedJob(c, ctx, sub, insRes)
	c.Assert(row.State, qt.Equals, rivertype.JobStateCompleted)
	c.Assert(row.Errors, qt.HasLen, 0)
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
	riverClient, username := setupWorkers(c, ctx, database, upgradeManager, sqlDB, 3, 1)

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

	insRes, err := riverClient.Insert(
		ctx,
		UpgradeToArgs{
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
	riverClient, username := setupWorkers(c, ctx, database, upgradeManager, sqlDB, 1, 3)

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
	riverClient, username := setupWorkers(c, ctx, database, upgradeManager, sqlDB, 2, 2)

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
	}, &river.InsertOpts{MaxAttempts: 1})
	c.Assert(err, qt.IsNil)

	row := waitForFinalisedJob(c, ctx, sub, insRes)
	c.Assert(row.State, qt.Equals, rivertype.JobStateCompleted)
	c.Assert(row.Errors, qt.HasLen, 0)
}

// This test simulates a crash of the supervisor between the migration and upgrade steps.
// And ensures that when retried, the migration job picks up where it left off uniquely
// resulting in 2 attempts.
//
// It relies on the fact that jobs are inserted in an ascending postgres sequence
// as we don't know the job id of the supervisor ahead of time otherwise. So,
// as we know it'll be the first job inserted, it's job id should always be 1.
func TestUpgradeToWorker_SupervisorResumesSuccessfullyAfterCrash(t *testing.T) {
	c := qt.New(t)
	ctx := c.Context()

	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	database := setupTestDB(c)
	sqlDB, err := database.SqlDB()
	c.Assert(err, qt.IsNil)

	upgradeManager := NewMockUpgradeManager(ctrl)

	supervisingJobId := int64(1)

	// Setup migrate job to retry up to 3 times, we're expecting 2 after the supervising job crash.
	riverClient, username := setupWorkers(c, ctx, database, upgradeManager, sqlDB, 3, 1)

	attempt := 0
	upgradeManager.EXPECT().
		MigrateModel(gomock.Any(), gomock.Any(), "model-uuid", "target-controller").
		DoAndReturn(func(context.Context, *openfga.User, string, string) error {
			attempt++
			if attempt == 1 {
				// We sneakily cancel the supervisor (lol) to simulate a crash.
				// As it's context has cancelled now, when it attempts to wait
				// for the migrate job to finish it is going to error out.
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
	}, &river.InsertOpts{MaxAttempts: 1})
	c.Assert(err, qt.IsNil)

	var supervisingJobFailureUpdate *rivertype.JobRow

loop:
	for {
		select {
		case event := <-sub:
			if event.Job.ID != supervisingJobId {
				continue loop
			}
			// We've caught the suppervising job entering "some state".
			// Capture it, and break out.
			supervisingJobFailureUpdate = event.Job
			break loop
		case <-ctx.Done():
			c.Fatal("timed out waiting for job failed event")
		}
	}

	// At this point, our job is cancelled and we'll see it as "completed".
	c.Assert(supervisingJobFailureUpdate.State, qt.Equals, rivertype.JobStateCompleted)
	// We're expecting to capture this specific singular job, and upon restarting the supervisor
	// we should see the supervisor succeeds and the attempts increase for this job.
	// We're pureposefully asking for more than we expect to ensure no other jobs are lurking.
	params := river.NewJobListParams().Kinds(migrationWorkerArgs{}.Kind()).First(10)
	listRes, err := riverClient.JobList(ctx, params)
	c.Assert(err, qt.IsNil)
	c.Assert(listRes.Jobs, qt.HasLen, 1)
	// WIP
}

func setupWorkers(
	c *qt.C,
	ctx context.Context,
	database *db.Database,
	upgradeManager UpgradeManager,
	sqlDB *sql.DB,
	migrateRetryCount int,
	upgradeRetryCount int,
) (*river.Client[*sql.Tx], string) {
	// Prepare identity needed by migrationWorker.
	u, err := dbmodel.NewIdentity("ash@catchum.com")
	c.Assert(err, qt.IsNil)
	err = database.GetIdentity(c.Context(), u)
	c.Assert(err, qt.IsNil)

	openfgaClient := &openfga.OFGAClient{}
	migrationW, err := newMigrationWorker(openfgaClient, database, upgradeManager)
	c.Assert(err, qt.IsNil)
	upgradeW, err := newUpgradeWorker(upgradeManager)
	c.Assert(err, qt.IsNil)
	upgradeToW := newUpgradeToWorker(migrateRetryCount, upgradeRetryCount)

	workers := river.NewWorkers()
	c.Assert(river.AddWorkerSafely(workers, migrationW), qt.IsNil)
	c.Assert(river.AddWorkerSafely(workers, upgradeW), qt.IsNil)
	c.Assert(river.AddWorkerSafely(workers, upgradeToW), qt.IsNil)

	riverClient, err := river.NewClient(riverdatabasesql.New(sqlDB), &river.Config{
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
