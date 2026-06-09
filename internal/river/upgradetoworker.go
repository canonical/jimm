// Copyright 2026 Canonical.

package river

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/riverqueue/river"

	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/openfga"
	"github.com/canonical/jimm/v3/internal/rivertypes"
)

const (
	upgradeToMigrationStep = "migration"
	upgradeToUpgradeStep   = "upgrade"

	upgradeFlowRetryAfterFirstFailure  = 30 * time.Second
	upgradeFlowRetryAfterSecondFailure = 2 * time.Minute
)

type upgradeToRetryFunc func(*river.Job[rivertypes.UpgradeToArgs]) time.Time

type stageError struct {
	stage string
	err   error
}

// Error implements the error interface.
func (e stageError) Error() string {
	return e.stage + " failed: " + e.err.Error()
}

func newUpgradeToWorker(openfgaClient *openfga.OFGAClient, store Store, upgradeManager UpgradeManager, nextRetry upgradeToRetryFunc) (*upgradeToWorker, error) {
	if openfgaClient == nil {
		return nil, errors.New("openfgaClient is required")
	}
	if store == nil {
		return nil, errors.New("store is required")
	}
	if upgradeManager == nil {
		return nil, errors.New("upgradeManager is required")
	}

	return &upgradeToWorker{
		openfgaClient:  openfgaClient,
		store:          store,
		upgradeManager: upgradeManager,
		nextRetry:      nextRetry,
	}, nil
}

type upgradeToWorker struct {
	river.WorkerDefaults[rivertypes.UpgradeToArgs]

	openfgaClient  *openfga.OFGAClient
	store          Store
	upgradeManager UpgradeManager
	nextRetry      upgradeToRetryFunc
}

// Timeout implements the [river.Worker] interface.
// To determine the timeout duration, we consider the maximum time
// for both the migration and upgrade steps, including retries.
func (w *upgradeToWorker) Timeout(*river.Job[rivertypes.UpgradeToArgs]) time.Duration {
	return 20 * time.Minute
}

// NextRetry implements the [river.Worker] interface.
// It determines the next retry time based on the attempt number,
// allowing for a longer delay after the second failure.
func (w *upgradeToWorker) NextRetry(job *river.Job[rivertypes.UpgradeToArgs]) time.Time {
	if w.nextRetry != nil {
		return w.nextRetry(job)
	}

	return defaultUpgradeToNextRetry(job)
}

func defaultUpgradeToNextRetry(job *river.Job[rivertypes.UpgradeToArgs]) time.Time {
	delay := upgradeFlowRetryAfterSecondFailure
	if job.Attempt == 1 {
		delay = upgradeFlowRetryAfterFirstFailure
	}
	return time.Now().Add(delay)
}

// Work implements the [river.Worker] interface.
//
// Each upgradeTo job uses River resumable steps so migration and upgrade share
// a single retry budget. If the job fails after migration completed, the next
// attempt resumes directly at the upgrade step.
func (w *upgradeToWorker) Work(ctx context.Context, job *river.Job[rivertypes.UpgradeToArgs]) error {
	client := river.ClientFromContext[*sql.Tx](ctx)

	// Step 1: Migrate the model to the target controller.
	river.ResumableStep(ctx, upgradeToMigrationStep, nil, func(ctx context.Context) error {
		if err := setUpgradeToJobInfo(ctx, client, job, fmt.Sprintf("Migrating model to controller %s", job.Args.TargetControllerName)); err != nil {
			return err
		}

		u := &dbmodel.Identity{Name: job.Args.Username}
		if err := w.store.FetchIdentity(ctx, u); err != nil {
			if persistErr := setUpgradeToJobInfo(ctx, client, job, "Migration failed"); persistErr != nil {
				return persistErr
			}
			return stageError{stage: upgradeToMigrationStep, err: err}
		}

		if err := w.upgradeManager.MigrateModel(
			ctx,
			openfga.NewUser(u, w.openfgaClient),
			job.Args.ModelUUID,
			job.Args.TargetControllerName,
		); err != nil {
			if persistErr := setUpgradeToJobInfo(ctx, client, job, "Migration failed"); persistErr != nil {
				return persistErr
			}
			return stageError{stage: upgradeToMigrationStep, err: err}
		}

		return setUpgradeToJobInfo(ctx, client, job, "Migration completed")
	})

	// Step 2: Upgrade the model to the target version.
	river.ResumableStep(ctx, upgradeToUpgradeStep, nil, func(ctx context.Context) error {
		if err := setUpgradeToJobInfo(ctx, client, job, fmt.Sprintf("Upgrading model to version %s", job.Args.TargetVersion)); err != nil {
			return err
		}

		if err := w.upgradeManager.UpgradeModel(ctx, job.Args.ModelUUID, job.Args.TargetVersion); err != nil {
			if persistErr := setUpgradeToJobInfo(ctx, client, job, "Upgrade failed"); persistErr != nil {
				return persistErr
			}
			return stageError{stage: upgradeToUpgradeStep, err: err}
		}

		return setUpgradeToJobInfo(ctx, client, job, "Upgrade completed")
	})

	return nil
}

// setUpgradeToJobInfo updates the job output with the provided info message.
func setUpgradeToJobInfo(ctx context.Context, client *river.Client[*sql.Tx], job *river.Job[rivertypes.UpgradeToArgs], info string) error {
	updatedJob, err := client.JobUpdate(ctx, job.ID, &river.JobUpdateParams{
		Output: rivertypes.UpgradeToOutput{Info: info},
	})
	if err != nil {
		return fmt.Errorf("failed to persist upgrade-to output: %w", err)
	}

	job.JobRow = updatedJob
	return nil
}
