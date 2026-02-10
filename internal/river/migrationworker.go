// Copyright 2026 Canonical.

package river

import (
	"context"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/openfga"
)

// newMigrationWorker creates a new upgradeMigrationWorker.
func newMigrationWorker(openfgaClient *openfga.OFGAClient, store Store, upgradeManager UpgradeManager) (*migrationWorker, error) {
	if openfgaClient == nil {
		return nil, errors.E("openfgaClient is required")
	}
	if store == nil {
		return nil, errors.E("store is required")
	}
	if upgradeManager == nil {
		return nil, errors.E("upgradeManager is required")
	}

	return &migrationWorker{
		openfgaClient:  openfgaClient,
		store:          store,
		upgradeManager: upgradeManager,
	}, nil
}

// migrationWorkerArgs defines the arguments for the migrationWorker job.
type migrationWorkerArgs struct {
	Username string `json:"username"`
	// UUID is the model UUID to migrate. We treat this as unique to prevent
	// multiple concurrent migrations of the same model to many controllers.
	UUID                 string `json:"uuid" river:"unique"`
	TargetControllerName string `json:"target_controller_name"`
}

// Kind implements the [river.JobArgs] interface.
func (migrationWorkerArgs) Kind() string { return "migrate-model" }

// InsertOpts implements the [river.JobArgsWithInsertOpts] interface.
//
// A job is considered unique by it's model uuid argument, and if it hasn't reached a completed state.
// Once it reaches completed, this job may be launched for the same model uuid again.
func (migrationWorkerArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		UniqueOpts: river.UniqueOpts{
			ByArgs: true,
			ByState: []rivertype.JobState{
				rivertype.JobStateAvailable,
				rivertype.JobStatePending,
				rivertype.JobStateRunning,
				rivertype.JobStateRetryable,
				rivertype.JobStateScheduled,
			},
		},
	}
}

type migrationWorker struct {
	// An embedded WorkerDefaults sets up default methods to fulfill the rest of
	// the Worker interface:
	river.WorkerDefaults[migrationWorkerArgs]

	openfgaClient  *openfga.OFGAClient
	store          Store
	upgradeManager UpgradeManager
}

// Work implements the [river.Worker] interface.
func (w *migrationWorker) Work(ctx context.Context, job *river.Job[migrationWorkerArgs]) error {
	u := &dbmodel.Identity{Name: job.Args.Username}
	if err := w.store.FetchIdentity(ctx, u); err != nil {
		return err
	}

	if err := w.upgradeManager.MigrateModel(ctx, openfga.NewUser(u, w.openfgaClient), job.Args.UUID, job.Args.TargetControllerName); err != nil {
		return err
	}

	return nil
}
