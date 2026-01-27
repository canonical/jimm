package river

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

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

	insRes, err := riverClient.Insert(ctx, UpgradeToArgs{
		ModelUUID:            "model-uuid",
		TargetVersion:        version.MustParse("2.0.0"),
		Username:             username,
		TargetControllerName: "target-controller",
	}, &river.InsertOpts{MaxAttempts: 1})
	c.Assert(err, qt.IsNil)

	row := waitForFinalisedJob(c, ctx, riverClient, insRes.Job.ID)
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
			switch attempt {
			case 1:
				return errors.New("unexpected-error")
			case 2:
				return errors.New("unexpected-error")
			case 3:
				return errors.New("migration-failed3")
			default:
				return errors.New("unexpected-error")
			}
		}).
		MinTimes(3)

	insRes, err := riverClient.Insert(
		ctx,
		UpgradeToArgs{
			ModelUUID:            "model-uuid",
			TargetVersion:        version.MustParse("2.0.0"),
			Username:             username,
			TargetControllerName: "target-controller",
		}, &river.InsertOpts{MaxAttempts: 1})
	c.Assert(err, qt.IsNil)

	row := waitForFinalisedJob(c, ctx, riverClient, insRes.Job.ID)
	c.Assert(row.State, qt.Equals, rivertype.JobStateDiscarded)
	c.Assert(len(row.Errors) > 0, qt.IsTrue)

	// Ensure we capture the last error only from the migrate job, and that it is surfaced to the upgrade to job.
	upgradeToJobFinalError := row.Errors[len(row.Errors)-1].Error
	c.Assert(upgradeToJobFinalError, qt.Equals, "migration-failed3")
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
			switch attempt {
			case 1:
				return errors.New("unexpected-error")
			case 2:
				return errors.New("unexpected-error")
			case 3:
				return errors.New("upgrade-failed3")
			default:
				return errors.New("unexpected-error")
			}
		}).
		MinTimes(3)

	insRes, err := riverClient.Insert(ctx, UpgradeToArgs{
		ModelUUID:            "model-uuid",
		TargetVersion:        version.MustParse("2.0.0"),
		Username:             username,
		TargetControllerName: "target-controller",
	}, &river.InsertOpts{MaxAttempts: 1})
	c.Assert(err, qt.IsNil)

	row := waitForFinalisedJob(c, ctx, riverClient, insRes.Job.ID)
	c.Assert(row.State, qt.Equals, rivertype.JobStateDiscarded)
	c.Assert(len(row.Errors) > 0, qt.IsTrue)

	// Ensure we capture the last error only from the upgrade job, and that it is surfaced to the upgrade to job.
	upgradeToJobFinalError := row.Errors[len(row.Errors)-1].Error
	c.Assert(upgradeToJobFinalError, qt.Equals, "upgrade-failed3")
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

	insRes, err := riverClient.Insert(ctx, UpgradeToArgs{
		ModelUUID:            "model-uuid",
		TargetVersion:        version.MustParse("2.0.0"),
		Username:             username,
		TargetControllerName: "target-controller",
	}, &river.InsertOpts{MaxAttempts: 1})
	c.Assert(err, qt.IsNil)

	row := waitForFinalisedJob(c, ctx, riverClient, insRes.Job.ID)
	c.Assert(row.State, qt.Equals, rivertype.JobStateCompleted)
	c.Assert(row.Errors, qt.HasLen, 0)
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
	c.Cleanup(func() { _ = riverClient.Stop(ctx) })

	return riverClient, u.Name
}

func waitForFinalisedJob(c *qt.C, ctx context.Context, client *river.Client[*sql.Tx], jobID int64) *rivertype.JobRow {
	for i := 0; i < 20; i++ {
		row, err := client.JobGet(ctx, jobID)
		c.Assert(err, qt.IsNil)
		if row.FinalizedAt != nil {
			return row
		}

		select {
		case <-ctx.Done():
			c.Fatalf("context done while waiting for job %d to finalize: %v", jobID, ctx.Err())
		case <-time.After(1 * time.Second):
		}
	}

	c.Fatalf("timed out waiting for job %d to finalize", jobID)
	return nil
}
