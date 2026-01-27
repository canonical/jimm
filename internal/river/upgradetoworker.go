package river

import (
	"context"
	"database/sql"
	"errors"

	"github.com/juju/version/v2"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
)

func newUpgradeToWorker(migrateRetries int, upgradeRetries int) *upgradeToWorker {
	return &upgradeToWorker{
		migrateRetries: migrateRetries,
		upgradeRetries: upgradeRetries,
	}
}

type UpgradeToArgs struct {
	ModelUUID            string         `json:"model-uuid"`
	TargetVersion        version.Number `json:"target-version"`
	Username             string         `json:"username"`
	TargetControllerName string         `json:"target_controller_name"`
}

func (UpgradeToArgs) Kind() string { return "upgrade-to" }

type upgradeToWorker struct {
	river.WorkerDefaults[UpgradeToArgs]

	// migrateRetries is the number of times to retry the migration step.
	migrateRetries int
	// upgradeRetries is the number of times to retry the upgrade step.
	upgradeRetries int
}

func (w *upgradeToWorker) Work(ctx context.Context, job *river.Job[UpgradeToArgs]) error {
	client := river.ClientFromContext[*sql.Tx](ctx)

	// PR Note:
	// Subscribe to job events immediately to prevent duplicate completion race as we'll catch it when inserting it
	// in a moment.
	//
	// River's initial buffer size for the subscription channel is 1000 [river.subscribeChanSizeDefault].
	// River does the following when pushing events:
	// select {
	// case sub.Chan <- event:
	// default:
	// }
	// Which can lead to dropped events. However, since we check the job state after insertion, we will not miss
	// any completions and process it in a timely manner.
	//
	// We could poll the job, as well as use subscriptions for quick wake ups between polls, but that adds complexity for
	// what might be an impossible scenario for us.
	//
	// It would look something like: tick on X seconds, and pull from ticker & sub channel together, whichever is first determines
	// if the job has completed. This covers us for dropped subscriptions.
	//
	// Open for discussion within the PR.
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
			},
		},
	)
	if err != nil {
		return err
	}

	if err := w.waitForJobToFinalise(migRes, eventCh); err != nil {
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
			},
		},
	)
	if err != nil {
		return err
	}

	if err := w.waitForJobToFinalise(upgradeRes, eventCh); err != nil {
		return err
	}

	// All done.
	return nil
}

func (w *upgradeToWorker) waitForJobToFinalise(result *rivertype.JobInsertResult, eventCh <-chan *river.Event) error {
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
	} else {
		for event := range eventCh {
			if event.Job.ID == result.Job.ID {
				if event.Job.FinalizedAt != nil {
					switch event.Kind {
					// Because we've finalised, this isn't an attempt failure, but the final state.
					case river.EventKindJobFailed:
						// Job failed, return the last error.
						if len(event.Job.Errors) != 0 {
							return errors.New(event.Job.Errors[len(event.Job.Errors)-1].Error)
						}
					case river.EventKindJobCancelled:
						return errors.New("job was cancelled")
					case river.EventKindJobCompleted:
						// Completed successfully.
						return nil
					}
				}
			}
		}
	}
	return nil
}
