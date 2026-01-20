package river

import (
	"context"

	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/openfga"
	"github.com/riverqueue/river"
)

// UpgradeManagerMigrationWorker defines the methods required from this manager for the migration worker.
type UpgradeManagerMigrationWorker interface {
	// MigrateModel migrates a model to a new controller without upgrading the model's agent.
	MigrateModel(ctx context.Context, user *openfga.User, modelUUID string, targetControllerName string) error
}

// newUpgradeMigrationWorker creates a new upgradeMigrationWorker.
func newUpgradeMigrationWorker(upgradeManager UpgradeManagerMigrationWorker) (*upgradeMigrationWorker, error) {
	if upgradeManager == nil {
		return nil, errors.E("upgradeManager is required")
	}

	return &upgradeMigrationWorker{
		upgradeManager: upgradeManager,
	}, nil
}

// UpgradeMigrationWorker defines the arguments for the upgradeMigrationWorker job.
type UpgradeMigrationWorker struct {
	User                 *openfga.User `json:"user"` // TODO: Will this work without JSON tags Alex? How will it serialise the user between instances?
	UUID                 string        `json:"uuid"`
	TargetControllerName string        `json:"target_controller_name"`
}

// Kind implements the [river.JobArgs] interface.
func (UpgradeMigrationWorker) Kind() string { return "upgrade-migration" }

type upgradeMigrationWorker struct {
	// An embedded WorkerDefaults sets up default methods to fulfill the rest of
	// the Worker interface:
	river.WorkerDefaults[UpgradeMigrationWorker]

	upgradeManager UpgradeManagerMigrationWorker
}

// Work implements the [river.Worker] interface.
func (w *upgradeMigrationWorker) Work(ctx context.Context, job *river.Job[UpgradeMigrationWorker]) error {
	if err := w.upgradeManager.MigrateModel(ctx, job.Args.User, job.Args.UUID, job.Args.TargetControllerName); err != nil {
		return err
	}

	return nil
}
