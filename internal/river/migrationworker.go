package river

import (
	"context"

	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/openfga"
	"github.com/juju/version/v2"
	"github.com/riverqueue/river"
)

// UpgradeToManager defines the interface for managing upgrade operations.
type UpgradeToManager interface {
	UpgradeTo(ctx context.Context, user *openfga.User, modelUUID string, targetVersion version.Number) (version.Number, error)
}

func newUpgradeToWorker(migrationManager UpgradeToManager) (*upgradeToWorker, error) {
	if migrationManager == nil {
		return nil, errors.E("migrationManager is required")
	}

	return &upgradeToWorker{
		migrationManager: migrationManager,
	}, nil
}

// UpgradeToArgs defines the arguments for the upgradeToWorker job.
type UpgradeToArgs struct {
	UUID string `json:"uuid"`
}

// Kind returns the kind of the job.
func (UpgradeToArgs) Kind() string { return "migration" }

type upgradeToWorker struct {
	// An embedded WorkerDefaults sets up default methods to fulfill the rest of
	// the Worker interface:
	river.WorkerDefaults[UpgradeToArgs]

	migrationManager UpgradeToManager
}

// Work performs the upgrade operation receiving the job with UpgradeToArgs.
func (w *upgradeToWorker) Work(ctx context.Context, job *river.Job[UpgradeToArgs]) error {
	_, err := w.migrationManager.UpgradeTo(ctx, nil, job.Args.UUID, version.Number{})
	if err != nil {
		return err
	}
	return nil
}
