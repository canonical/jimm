// Copyright 2026 Canonical.

package river

import (
	"context"

	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/juju/version/v2"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
)

func newUpgradeWorker(upgradeManager UpgradeManager) (*upgradeWorker, error) {
	if upgradeManager == nil {
		return nil, errors.E("upgradeManager is required")
	}

	return &upgradeWorker{
		upgradeManager: upgradeManager,
	}, nil
}

// upgradeWorkerArgs defines the arguments for the upgradeWorker job.
type upgradeWorkerArgs struct {
	// ModelUUID is the model UUID to migrate. We treat this as unique to prevent
	// multiple concurrent migrations of the same model to many controllers.
	ModelUUID     string         `json:"model-uuid" river:"unique"`
	TargetVersion version.Number `json:"target-version"`
}

// Kind returns the kind of the job.
func (upgradeWorkerArgs) Kind() string { return "upgrade-model" }

// InsertOpts implements the [river.JobArgsWithInsertOpts] interface.
//
// A job is considered unique by it's model uuid argument, and if it hasn't reached a completed state.
// Once it reaches completed, this job may be launched for the same model uuid again.
func (upgradeWorkerArgs) InsertOpts() river.InsertOpts {
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

type upgradeWorker struct {
	// An embedded WorkerDefaults sets up default methods to fulfill the rest of
	// the Worker interface:
	river.WorkerDefaults[upgradeWorkerArgs]

	upgradeManager UpgradeManager
}

// Work performs the upgrade operation receiving the job with UpgradeArgs.
func (w *upgradeWorker) Work(ctx context.Context, job *river.Job[upgradeWorkerArgs]) error {
	err := w.upgradeManager.UpgradeModel(ctx, job.Args.ModelUUID, job.Args.TargetVersion)
	if err != nil {
		return err
	}
	return nil
}
