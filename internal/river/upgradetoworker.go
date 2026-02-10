// Copyright 2026 Canonical.

package river

import (
	"context"
	"database/sql"
	"errors"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/canonical/jimm/v3/internal/rivertypes"
)

// awaitCompletionFunc is a function that waits for a job to finalise.
type awaitCompletionFunc func(ctx context.Context, result *rivertype.JobInsertResult, eventCh <-chan *river.Event) error

func newUpgradeToWorker(migrateRetries int, upgradeRetries int, awaitFunc awaitCompletionFunc) *upgradeToWorker {
	return &upgradeToWorker{
		migrateRetries:  migrateRetries,
		upgradeRetries:  upgradeRetries,
		awaitCompletion: awaitFunc,
	}
}

type upgradeToWorker struct {
	river.WorkerDefaults[rivertypes.UpgradeToArgs]

	// migrateRetries is the number of times to retry the migration step.
	migrateRetries int
	// upgradeRetries is the number of times to retry the upgrade step.
	upgradeRetries int
	// awaitCompletion is a function that waits for a job to finalise.
	awaitCompletion awaitCompletionFunc
}

// Work implements the [river.Worker] interface.
//
// Each upgradeTo job acts as a orchestrator of two child jobs, starting them and waiting for their
// completion sequentially.
//
// River's unique-job args and states are setup to ensure that in-progress jobs are not re-inserted,
// keyed by the model UUID and job state.
//
// Each child job is expected to be idempotent so that in certain edge cases where an orchestrator
// restart would cause re-insertion of a completed job, no changes are made. (Like between the completion
// of the migration job and the insertion of the upgrade job).
func (w *upgradeToWorker) Work(ctx context.Context, job *river.Job[rivertypes.UpgradeToArgs]) error {
	client := river.ClientFromContext[*sql.Tx](ctx)

	eventCh, cancel := client.Subscribe(
		river.EventKindJobCompleted,
		river.EventKindJobCancelled,
		river.EventKindJobFailed,
	)
	defer cancel()

	migrateInsertResponse, err := client.Insert(
		ctx,
		migrationWorkerArgs{
			Username:             job.Args.Username,
			UUID:                 job.Args.ModelUUID,
			TargetControllerName: job.Args.TargetControllerName,
		},
		&river.InsertOpts{
			MaxAttempts: w.migrateRetries,
		},
	)
	if err != nil {
		return err
	}

	if err := w.awaitCompletion(ctx, migrateInsertResponse, eventCh); err != nil {
		return err
	}

	upgradeInsertResponse, err := client.Insert(
		ctx,
		upgradeWorkerArgs{
			ModelUUID:     job.Args.ModelUUID,
			TargetVersion: job.Args.TargetVersion,
		},
		&river.InsertOpts{
			MaxAttempts: w.upgradeRetries,
		},
	)
	if err != nil {
		return err
	}

	if err := w.awaitCompletion(ctx, upgradeInsertResponse, eventCh); err != nil {
		return err
	}

	// All done.
	return nil
}

// waitForJobToFinalise waits for the job to finalise, that is, a job that will no longer
// be retried but could have succeeded or failed after all attempts. It does so by checking
// the event channel for updates.
//
// If the job has been inserted already on a previous attempt, it checks if it's finalised already,
// and if not, waits for it to do so.
func waitForJobToFinalise(ctx context.Context, result *rivertype.JobInsertResult, eventCh <-chan *river.Event) error {
	// It may be a duplicate, so check if it has finalised. If not, wait for it to do so.
	if result.Job.FinalizedAt != nil {
		// It has finalised, check it's state, if it failed return error.
		if len(result.Job.Errors) != 0 {
			return errors.New(result.Job.Errors[len(result.Job.Errors)-1].Error)
		}
		return nil
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case event, ok := <-eventCh:
			if !ok {
				return errors.New("event channel closed unexpectedly")
			}

			if event.Job.ID != result.Job.ID || event.Job.FinalizedAt == nil {
				continue
			}

			switch event.Kind {
			// Because we've finalised, this isn't an attempt failure, but the final state.
			case river.EventKindJobFailed:
				// Job failed, return the last error.
				if len(event.Job.Errors) != 0 {
					return errors.New(event.Job.Errors[len(event.Job.Errors)-1].Error)
				}
				return errors.New("job failed without error details")
			case river.EventKindJobCancelled:
				return errors.New("job was cancelled")
			case river.EventKindJobCompleted:
				// Completed successfully.
				return nil
			}

		}
	}
}
