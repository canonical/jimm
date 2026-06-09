// Copyright 2026 Canonical.

package river

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	qt "github.com/frankban/quicktest"
	"github.com/juju/version/v2"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertest"
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
		setupWorkerParams{},
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
	output := getUpgradeToJobOutput(c, ctx, riverClient, insRes.Job.ID)
	c.Assert(output.Info, qt.Equals, "Upgrade completed")
}

func TestUpgradeToWorker_SuccessCanBeUpgradedToAgain(t *testing.T) {
	c := qt.New(t)
	ctx := c.Context()

	testDeps := setupIntegrationTest(
		c,
		setupWorkerParams{},
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
	c.Assert(row.State, qt.Equals, rivertype.JobStateCompleted)
	c.Assert(row.Errors, qt.HasLen, 0)

	// We can see two distinct jobs now because the first one entered a completed state.
	c.Assert(insRes.UniqueSkippedAsDuplicate, qt.IsFalse)

	listRes, err := riverClient.JobList(ctx, river.NewJobListParams().Kinds(rivertypes.UpgradeToJobKind).First(10))
	c.Assert(err, qt.IsNil)
	c.Assert(listRes.Jobs[0].State, qt.Equals, rivertype.JobStateCompleted)
	c.Assert(listRes.Jobs, qt.HasLen, 2)
}

func TestUpgradeToWorker_MigrationFails(t *testing.T) {
	c := qt.New(t)
	ctx := c.Context()

	testDeps := setupIntegrationTest(
		c,
		setupWorkerParams{},
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
		Times(3)

	sub, cancel := riverClient.Subscribe(river.EventKindJobFailed)
	c.Cleanup(cancel)

	insRes, err := riverClient.Insert(ctx, rivertypes.UpgradeToArgs{
		ModelUUID:            "model-uuid",
		TargetVersion:        version.MustParse("2.0.0"),
		Username:             username,
		TargetControllerName: "target-controller",
	}, nil)
	c.Assert(err, qt.IsNil)

	row := waitForFinalisedJob(c, ctx, sub, insRes.Job.ID)
	c.Assert(row.State, qt.Equals, rivertype.JobStateDiscarded)
	c.Assert(row.Attempt, qt.Equals, 3)
	upgradeToJobFinalError := row.Errors[len(row.Errors)-1].Error
	c.Assert(upgradeToJobFinalError, qt.Equals, "migration failed: unexpected-error-3")
	output := getUpgradeToJobOutput(c, ctx, riverClient, insRes.Job.ID)
	c.Assert(output.Info, qt.Equals, "Migration failed")
}

func TestUpgradeToWorker_UpgradeFails(t *testing.T) {
	c := qt.New(t)
	ctx := c.Context()

	testDeps := setupIntegrationTest(
		c,
		setupWorkerParams{},
	)

	riverClient := testDeps.riverClient
	username := testDeps.identity
	upgradeManager := testDeps.mockUpgradeManager

	attempt := 0
	upgradeManager.EXPECT().
		UpgradeModel(gomock.Any(), "model-uuid", version.MustParse("2.0.0")).
		DoAndReturn(func(context.Context, string, version.Number) error {
			attempt++
			return fmt.Errorf("unexpected-error-%d", attempt)
		}).
		Times(3)

	sub, cancel := riverClient.Subscribe(river.EventKindJobFailed)
	c.Cleanup(cancel)

	insRes, err := riverClient.Insert(ctx, rivertypes.UpgradeToArgs{
		ModelUUID:            "model-uuid",
		TargetVersion:        version.MustParse("2.0.0"),
		Username:             username,
		TargetControllerName: "target-controller",
	}, rivertest.ResumableStepAfter(&river.InsertOpts{}, upgradeToMigrationStep))
	c.Assert(err, qt.IsNil)

	row := waitForFinalisedJob(c, ctx, sub, insRes.Job.ID)
	c.Assert(row.State, qt.Equals, rivertype.JobStateDiscarded)
	c.Assert(row.Attempt, qt.Equals, 3)
	upgradeToJobFinalError := row.Errors[len(row.Errors)-1].Error
	c.Assert(upgradeToJobFinalError, qt.Equals, "upgrade failed: unexpected-error-3")
	output := getUpgradeToJobOutput(c, ctx, riverClient, insRes.Job.ID)
	c.Assert(output.Info, qt.Equals, "Upgrade failed")
}

func TestUpgradeToWorker_RetrySkipsCompletedMigration(t *testing.T) {
	c := qt.New(t)
	ctx := c.Context()

	testDeps := setupIntegrationTest(
		c,
		setupWorkerParams{},
	)

	riverClient := testDeps.riverClient
	username := testDeps.identity
	upgradeManager := testDeps.mockUpgradeManager

	upgradeManager.EXPECT().
		MigrateModel(gomock.Any(), gomock.Any(), "model-uuid", "target-controller").
		Return(nil).
		Times(1)

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
	c.Assert(row.Errors, qt.HasLen, 1)
	c.Assert(row.Errors[0].Error, qt.Equals, "upgrade failed: upgrade-transient")
	c.Assert(row.Attempt, qt.Equals, 2)
	output := getUpgradeToJobOutput(c, ctx, riverClient, insRes.Job.ID)
	c.Assert(output.Info, qt.Equals, "Upgrade completed")
}

func TestUpgradeToWorker_PersistsRunningMigrationOutput(t *testing.T) {
	c := qt.New(t)
	ctx := c.Context()

	migrationStarted := make(chan struct{})
	allowMigration := make(chan struct{})

	testDeps := setupIntegrationTest(
		c,
		setupWorkerParams{},
	)

	riverClient := testDeps.riverClient
	username := testDeps.identity
	upgradeManager := testDeps.mockUpgradeManager

	upgradeManager.EXPECT().
		MigrateModel(gomock.Any(), gomock.Any(), "model-uuid", "target-controller").
		DoAndReturn(func(context.Context, *openfga.User, string, string) error {
			close(migrationStarted)
			<-allowMigration
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

	<-migrationStarted

	output := getUpgradeToJobOutput(c, ctx, riverClient, insRes.Job.ID)
	c.Assert(output.Info, qt.Equals, "Migrating model to controller target-controller")

	close(allowMigration)

	row := waitForFinalisedJob(c, ctx, sub, insRes.Job.ID)
	c.Assert(row.State, qt.Equals, rivertype.JobStateCompleted)
	output = getUpgradeToJobOutput(c, ctx, riverClient, insRes.Job.ID)
	c.Assert(output.Info, qt.Equals, "Upgrade completed")
}

func TestUpgradeToWorker_StopAndCancelDoesNotLeaveJobRunning(t *testing.T) {
	c := qt.New(t)
	ctx := c.Context()

	migrationStarted := make(chan struct{})

	testDeps := setupIntegrationTest(
		c,
		setupWorkerParams{},
	)

	riverClient := testDeps.riverClient
	username := testDeps.identity
	upgradeManager := testDeps.mockUpgradeManager

	upgradeManager.EXPECT().
		MigrateModel(gomock.Any(), gomock.Any(), "model-uuid", "target-controller").
		DoAndReturn(func(ctx context.Context, _ *openfga.User, _, _ string) error {
			close(migrationStarted)
			<-ctx.Done()
			return ctx.Err()
		})

	insRes, err := riverClient.Insert(ctx, rivertypes.UpgradeToArgs{
		ModelUUID:            "model-uuid",
		TargetVersion:        version.MustParse("2.0.0"),
		Username:             username,
		TargetControllerName: "target-controller",
	}, nil)
	c.Assert(err, qt.IsNil)

	<-migrationStarted
	waitForJobState(c, ctx, riverClient, insRes.Job.ID, rivertype.JobStateRunning, rivertypes.UpgradeToJobKind)

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	c.Assert(riverClient.StopAndCancel(shutdownCtx), qt.IsNil)

	rootJob, err := riverClient.JobGet(ctx, insRes.Job.ID)
	c.Assert(err, qt.IsNil)
	c.Assert(rootJob.State, qt.Not(qt.Equals), rivertype.JobStateRunning)

	output := getUpgradeToJobOutput(c, ctx, riverClient, insRes.Job.ID)
	c.Assert(output.Info, qt.Not(qt.Equals), "")
}
