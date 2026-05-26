// Copyright 2026 Canonical.

package river

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

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

// Timeout implements the [river.Worker] interface.
// To determine the timeout duration, we consider the maximum time
// for both the migration and upgrade steps, including retries.
func (w *upgradeToWorker) Timeout(*river.Job[rivertypes.UpgradeToArgs]) time.Duration {
	return 20 * time.Minute
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
	supervisorOutput, err := loadUpgradeToSupervisorOutput(job)
	if err != nil {
		return err
	}

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

	supervisorOutput = withMigrationJobID(supervisorOutput, migrateInsertResponse.Job.ID)
	if err := persistUpgradeToSupervisorOutput(ctx, client, job.ID, supervisorOutput); err != nil {
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

	supervisorOutput = withUpgradeJobID(supervisorOutput, upgradeInsertResponse.Job.ID)
	if err := persistUpgradeToSupervisorOutput(ctx, client, job.ID, supervisorOutput); err != nil {
		return err
	}

	if err := w.awaitCompletion(ctx, upgradeInsertResponse, eventCh); err != nil {
		return err
	}

	// All done.
	return nil
}

// loadUpgradeToSupervisorOutput loads stored output from supervisor job, ensuring that any
// existing output is consistent with the job args.
//
// The output is expected to contain the child job IDs of any previously started child jobs,
// so that they can be tracked across restarts of the supervisor job.
func loadUpgradeToSupervisorOutput(job *river.Job[rivertypes.UpgradeToArgs]) (rivertypes.UpgradeToSupervisorOutput, error) {
	output := rivertypes.UpgradeToSupervisorOutput{
		ModelUUID:            job.Args.ModelUUID,
		TargetControllerName: job.Args.TargetControllerName,
	}

	if len(job.Output()) == 0 {
		return output, nil
	}

	var stored rivertypes.UpgradeToSupervisorOutput
	if err := json.Unmarshal(job.Output(), &stored); err != nil {
		return output, fmt.Errorf("failed to decode existing supervisor output: %w", err)
	}
	if stored.ModelUUID != "" && stored.ModelUUID != job.Args.ModelUUID {
		return output, fmt.Errorf("stored supervisor output model UUID %q does not match job args %q", stored.ModelUUID, job.Args.ModelUUID)
	}
	if stored.TargetControllerName != "" && stored.TargetControllerName != job.Args.TargetControllerName {
		return output, fmt.Errorf("stored supervisor output target controller %q does not match job args %q", stored.TargetControllerName, job.Args.TargetControllerName)
	}

	if stored.ModelUUID != "" {
		output.ModelUUID = stored.ModelUUID
	}
	if stored.TargetControllerName != "" {
		output.TargetControllerName = stored.TargetControllerName
	}
	output.MigrationJobID = stored.MigrationJobID
	output.UpgradeJobID = stored.UpgradeJobID
	output.UpdatedAt = stored.UpdatedAt

	return output, nil
}

func withMigrationJobID(output rivertypes.UpgradeToSupervisorOutput, jobID int64) rivertypes.UpgradeToSupervisorOutput {
	migrationJobID := jobID
	output.MigrationJobID = &migrationJobID
	output.UpdatedAt = time.Now().UTC()
	return output
}

func withUpgradeJobID(output rivertypes.UpgradeToSupervisorOutput, jobID int64) rivertypes.UpgradeToSupervisorOutput {
	upgradeJobID := jobID
	output.UpgradeJobID = &upgradeJobID
	output.UpdatedAt = time.Now().UTC()
	return output
}

func persistUpgradeToSupervisorOutput(ctx context.Context, client *river.Client[*sql.Tx], jobID int64, output rivertypes.UpgradeToSupervisorOutput) error {
	if _, err := client.JobUpdate(ctx, jobID, &river.JobUpdateParams{Output: output}); err != nil {
		return fmt.Errorf("failed to persist supervisor output: %w", err)
	}
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
