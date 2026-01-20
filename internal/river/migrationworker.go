package river

import (
	"context"

	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/openfga"
	"github.com/riverqueue/river"
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
	Username             string `json:"username"`
	UUID                 string `json:"uuid"`
	TargetControllerName string `json:"target_controller_name"`
}

// Kind implements the [river.JobArgs] interface.
func (migrationWorkerArgs) Kind() string { return "upgrade-migration" }

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
