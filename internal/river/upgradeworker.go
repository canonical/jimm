package river

import (
	"context"

	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/juju/version/v2"
	"github.com/riverqueue/river"
)

func newUpgradeWorker(upgradeManager UpgradeManager) (*upgradeWorker, error) {
	if upgradeManager == nil {
		return nil, errors.E("migrationManager is required")
	}

	return &upgradeWorker{
		upgradeManager: upgradeManager,
	}, nil
}

// upgradeArgs defines the arguments for the upgradeWorker job.
type upgradeArgs struct {
	// ModelUUID is the model UUID to migrate. We treat this as unique to prevent
	// multiple concurrent migrations of the same model to many controllers.
	ModelUUID     string         `json:"model-uuid" river:"unique"`
	TargetVersion version.Number `json:"target-version"`
}

// Kind returns the kind of the job.
func (upgradeArgs) Kind() string { return "upgrade" }

type upgradeWorker struct {
	// An embedded WorkerDefaults sets up default methods to fulfill the rest of
	// the Worker interface:
	river.WorkerDefaults[upgradeArgs]

	upgradeManager UpgradeManager
}

// Work performs the upgrade operation receiving the job with UpgradeArgs.
func (w *upgradeWorker) Work(ctx context.Context, job *river.Job[upgradeArgs]) error {
	err := w.upgradeManager.UpgradeModel(ctx, job.Args.ModelUUID, job.Args.TargetVersion)
	if err != nil {
		return err
	}
	return nil
}
