// Copyright 2026 Canonical.

package river

import (
	"context"
	"database/sql"
	"errors"

	"github.com/juju/version/v2"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
)

// waitForJobFinalisationFunc is a function that waits for a job to finalise.
type waitForJobFinalisationFunc func(ctx context.Context, result *rivertype.JobInsertResult, eventCh <-chan *river.Event) error

func newUpgradeToWorker(migrateRetries int, upgradeRetries int, finaliser waitForJobFinalisationFunc) *upgradeToWorker {
	return &upgradeToWorker{
		migrateRetries: migrateRetries,
		upgradeRetries: upgradeRetries,
		finaliser:      finaliser,
	}
}

// UpgradeToArgs are the arguments for the upgrade-to worker.
type UpgradeToArgs struct {
	ModelUUID            string         `json:"model-uuid" river:"unique"`
	TargetVersion        version.Number `json:"target-version"`
	Username             string         `json:"username"`
	TargetControllerName string         `json:"target_controller_name"`
}

// Kind implements the [river.JobArgs] interface.
func (UpgradeToArgs) Kind() string { return "upgrade-to" }

type upgradeToWorker struct {
	river.WorkerDefaults[UpgradeToArgs]

	// migrateRetries is the number of times to retry the migration step.
	migrateRetries int
	// upgradeRetries is the number of times to retry the upgrade step.
	upgradeRetries int
	// finaliser is a function that waits for a job to finalise.
	finaliser waitForJobFinalisationFunc
}

// Work implements the [river.Worker] interface.
func (w *upgradeToWorker) Work(ctx context.Context, job *river.Job[UpgradeToArgs]) error {
	client := river.ClientFromContext[*sql.Tx](ctx)

	eventCh, cancel := client.Subscribe(river.EventKindJobCompleted, river.EventKindJobCancelled, river.EventKindJobFailed)
	defer cancel()

	migRes, err := client.Insert(
		ctx,
		migrationWorkerArgs{
			Username:             job.Args.Username,
			UUID:                 job.Args.ModelUUID,
			TargetControllerName: job.Args.TargetControllerName,
		},
		&river.InsertOpts{
			MaxAttempts: w.migrateRetries,
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
		},
	)
	if err != nil {
		return err
	}

	if err := w.finaliser(ctx, migRes, eventCh); err != nil {
		return err
	}

	upgradeRes, err := client.Insert(
		ctx,
		upgradeArgs{
			ModelUUID:     job.Args.ModelUUID,
			TargetVersion: job.Args.TargetVersion,
		},
		&river.InsertOpts{
			MaxAttempts: w.upgradeRetries,
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
		},
	)
	if err != nil {
		return err
	}

	if err := w.finaliser(ctx, upgradeRes, eventCh); err != nil {
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
		//
		// TODO: We should report all errors from all attempts and their details (i.e., timestamps)
		// For now, simply take the last failure attempt.
		// This should be completed in Phase 3.
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
