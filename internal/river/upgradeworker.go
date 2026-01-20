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

// UpgradeArgs defines the arguments for the upgradeWorker job.
type UpgradeArgs struct {
	ModelUUID     string         `json:"model-uuid"`
	TargetVersion version.Number `json:"target-version"`
}

// Kind returns the kind of the job.
func (UpgradeArgs) Kind() string { return "upgrade" }

type upgradeWorker struct {
	// An embedded WorkerDefaults sets up default methods to fulfill the rest of
	// the Worker interface:
	river.WorkerDefaults[UpgradeArgs]

	upgradeManager UpgradeManager
}

// Work performs the upgrade operation receiving the job with UpgradeArgs.
func (w *upgradeWorker) Work(ctx context.Context, job *river.Job[UpgradeArgs]) error {
	err := w.upgradeManager.UpgradeModel(ctx, job.Args.ModelUUID, job.Args.TargetVersion)
	if err != nil {
		return err
	}
	return nil
}
