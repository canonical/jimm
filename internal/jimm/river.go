// Copyright 2024 Canonical Ltd.

package jimm

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/juju/zaputil"
	"github.com/juju/zaputil/zapctx"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivermigrate"
	"github.com/riverqueue/river/rivertype"
	"go.uber.org/zap"

	"github.com/canonical/jimm/internal/db"
	"github.com/canonical/jimm/internal/errors"
	"github.com/canonical/jimm/internal/jimm/workers"
	"github.com/canonical/jimm/internal/openfga"
	"github.com/canonical/jimm/internal/servermon"
)

// River is the struct that holds that Client connection to river.
type River struct {
	// Client is a pointer to the river client that is used to interact with the service.
	Client *river.Client[pgx.Tx]
	dbPool *pgxpool.Pool
	// MaxAttempts is how many times river would retry before a job is abandoned and set as
	// discarded. This includes the original and the retry attempts.
	MaxAttempts int
	// RetryPolicy determines when the next attempt for a failed job will be run.
	RetryPolicy river.ClientRetryPolicy
}

// registerJimmWorkers would register known workers safely and return a pointer to a river.workers struct that should be used in river creation.
func registerJimmWorkers(ctx context.Context, ofgaClient *openfga.OFGAClient, db *db.Database, jimm *JIMM) (*river.Workers, error) {
	workers := river.NewWorkers()
	if err := river.AddWorkerSafely(workers, &RiverAddModelWorker{OfgaClient: ofgaClient, Database: db, JIMM: jimm}); err != nil {
		return nil, err
	}
	return workers, nil
}

// Cleanup closes the pool on exit and waits for running jobs to finish before doing a graceful shutdown.
func (r *River) Cleanup(ctx context.Context) error {
	defer r.dbPool.Close()
	return r.Client.Stop(ctx)
}

func (r *River) doMigration(ctx context.Context) error {
	migrator := rivermigrate.New(riverpgxv5.New(r.dbPool), nil)
	tx, err := r.dbPool.Begin(ctx)
	if err != nil {
		return err
	}
	_, err = migrator.MigrateTx(ctx, tx, rivermigrate.DirectionUp, nil)
	if err != nil {
		zapctx.Error(ctx, "failed to apply DB migration", zap.Error(err))
		rollbackErr := tx.Rollback(ctx)
		if rollbackErr != nil {
			zapctx.Error(ctx, "failed to rollback DB migration", zap.Error(rollbackErr))
		}
		return err
	}
	if err = tx.Commit(ctx); err != nil {
		zapctx.Error(ctx, "failed to commit DB migration", zap.Error(err))
		return err
	}
	return nil
}

// Monitor listens on the failure channel and increments the prometheus counters for every failed job kind
func (r *River) Monitor() {
	var failedChan <-chan *river.Event
	var failedSubscribeCancel func()
	failedChan, failedSubscribeCancel = r.Client.Subscribe(river.EventKindJobFailed)
	defer failedSubscribeCancel()

	for {
		item := <-failedChan
		if item.Job.Attempt == item.Job.MaxAttempts && item.Job.FinalizedAt != nil {
			servermon.FailedJobsCount.WithLabelValues(item.Job.Kind).Inc()
		}
	}
}

// RiverConfig is a struct that represents the arguments to create the River instance.
type RiverConfig struct {
	// Config is an optional river config object that includes the retry policy,
	// the queue defaults and the maximum number of workers to create, as well as the workers themselves.
	Config *river.Config
	// DSN is JIMM Db's DSN, which is used to create river's PG connection pool.
	DSN string
	// MaxAttempts is how many times river would retry before a job is abandoned and set as
	// discarded. This includes the original and the retry attempts.
	MaxAttempts int
	// MaxWorkers configures the maximum number of workers to create per task/job type.
	MaxWorkers int
}

// NewRiver returns a new river instance after applying the needed migrations to the database.
// It will open a postgres connections pool that would be closed in the Cleanup routine.
func NewRiver(ctx context.Context, riverConfig RiverConfig, ofgaClient *openfga.OFGAClient, db *db.Database, jimm *JIMM) (_ *River, err error) {
	maxAttempts := max(1, riverConfig.MaxAttempts)
	workers, err := registerJimmWorkers(ctx, ofgaClient, db, jimm)
	if err != nil {
		return nil, err
	}
	if riverConfig.Config == nil {
		riverConfig.Config = &river.Config{
			RetryPolicy: &river.DefaultClientRetryPolicy{},
			Queues: map[string]river.QueueConfig{
				river.QueueDefault: {MaxWorkers: max(1, riverConfig.MaxWorkers)},
			},
			// Logger:  slog.Default(),
			Workers: workers,
		}
	}
	dbPool, err := pgxpool.New(ctx, riverConfig.DSN)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			dbPool.Close()
		}
	}()
	riverClient, err := river.NewClient(riverpgxv5.New(dbPool), riverConfig.Config)
	if err != nil {
		dbPool.Close()
		return nil, err
	}
	if err := riverClient.Start(ctx); err != nil {
		dbPool.Close()
		return nil, err
	}
	r := River{Client: riverClient, dbPool: dbPool, MaxAttempts: maxAttempts}
	err = r.doMigration(ctx)
	if err != nil {
		dbPool.Close()
		return nil, err
	}

	return &r, nil
}

// InsertJob performs the insertion function provided to add a job to River's job queue.
// If waitConfig is not nil, the function blocks until the job is finished, failed, canceled, times out, or the context is done.
func InsertJob(ctx context.Context, waitConfig *workers.WaitConfig, r *River, insertFunc func() (*rivertype.JobRow, error)) error {
	var completedChan <-chan *river.Event
	var completedSubscribeCancel func()
	var failedChan <-chan *river.Event
	var failedSubscribeCancel func()
	var otherChan <-chan *river.Event
	var otherSubscribeCancel func()

	if waitConfig != nil {
		completedChan, completedSubscribeCancel = r.Client.Subscribe(river.EventKindJobCompleted)
		defer completedSubscribeCancel()
		failedChan, failedSubscribeCancel = r.Client.Subscribe(river.EventKindJobFailed)
		defer failedSubscribeCancel()
		otherChan, otherSubscribeCancel = r.Client.Subscribe(river.EventKindJobCancelled, river.EventKindJobSnoozed)
		defer otherSubscribeCancel()
	}

	row, err := insertFunc()
	if err != nil {
		zapctx.Error(ctx, "failed to insert river job", zaputil.Error(err))
		return errors.E(err, "failed to insert river job")
	}
	if waitConfig != nil {
		for {
			select {
			case item := <-completedChan:
				if item.Job.ID == row.ID {
					return nil
				}
			case item := <-failedChan:
				if item.Job.ID == row.ID {
					if item.Job.Attempt == item.Job.MaxAttempts && item.Job.FinalizedAt != nil {
						return errors.E(fmt.Sprintf("river job %d failed after %d attempts at %s. failure reason %v", item.Job.ID, item.Job.Attempt, item.Job.FinalizedAt, item.Job.Errors))
					} else {
						zapctx.Warn(ctx, fmt.Sprintf("job %d failed in attempt %d with error %v, river will continue retrying!", item.Job.ID, item.Job.Attempt, item.Job.Errors[item.Job.Attempt-1]))
					}
				}
			case item := <-otherChan:
				if item.Job.ID == row.ID && item.Job.State == river.JobStateCancelled {
					return errors.E(fmt.Sprintf("river job %d was cancelled", item.Job.ID))
				}
			case <-time.After(waitConfig.Duration):
				return errors.E(fmt.Sprintf("timed out after %s waiting for river to process the job", waitConfig.Duration))
			case <-ctx.Done():
				return errors.E(ctx.Err())
			}
		}
	}
	return nil
}
