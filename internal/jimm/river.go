// Copyright 2024 Canonical Ltd.

package jimm

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/juju/names/v4"
	"github.com/juju/zaputil/zapctx"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivermigrate"
	"go.uber.org/zap"

	"github.com/canonical/jimm/internal/db"
	"github.com/canonical/jimm/internal/dbmodel"
	"github.com/canonical/jimm/internal/errors"
	"github.com/canonical/jimm/internal/openfga"
	ofganames "github.com/canonical/jimm/internal/openfga/names"
)

// RiverOpenFGAArgs holds the river job arguments for writing tuples to OpenFGA.
type RiverOpenFGAArgs struct {
	ControllerUUID string `json:"controller_uuid"`
	ModelId        uint   `json:"model_id"`
	ModelInfoUUID  string `json:"model_info_uuid"`
	OwnerName      string `json:"owner_name"`
}

// Kind is a string that uniquely identifies the type of job to be picked up by the appropriate river workers.
// This is required by river and must be provided on the job arguments struct to implement the JobArgs interface.
func (RiverOpenFGAArgs) Kind() string { return "OpenFGA" }

// OpenFGAWorker is the river worker that would run the job.
type RiverOpenFGAWorker struct {
	river.WorkerDefaults[RiverOpenFGAArgs]
	OfgaClient *openfga.OFGAClient
	Database   db.Database
}

// Work is the function executed by the worker when it picks up the job.
func (w *RiverOpenFGAWorker) Work(ctx context.Context, job *river.Job[RiverOpenFGAArgs]) error {
	controllerTag := names.NewControllerTag(job.Args.ControllerUUID)
	modelTag := names.NewModelTag(job.Args.ModelInfoUUID)

	owner := &dbmodel.User{Username: job.Args.OwnerName}
	err := w.Database.GetUser(ctx, owner)
	if err != nil {
		return err
	}
	if err := w.OfgaClient.AddControllerModel(
		ctx,
		controllerTag,
		modelTag,
	); err != nil {
		zapctx.Error(
			ctx,
			"failed to add controller-model relation",
			zap.String("controller", controllerTag.Id()),
			zap.String("model", modelTag.Id()),
		)
		return errors.E(err, "failed to add the controller-model relation from the river job.")
	}
	err = openfga.NewUser(owner, w.OfgaClient).SetModelAccess(ctx, names.NewModelTag(job.Args.ModelInfoUUID), ofganames.AdministratorRelation)
	if err != nil {
		zapctx.Error(
			ctx,
			"failed to add administrator relation",
			zap.String("user", owner.Tag().String()),
			zap.String("model", modelTag.Id()),
		)
		return errors.E(err, "failed to add the administrator relation from the river job.")
	}
	return nil
}

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

// RegisterJimmWorkers would register known workers safely and return a pointer to a river.workers struct that should be used in river creation.
func RegisterJimmWorkers(ctx context.Context, ofgaConn *openfga.OFGAClient, db *db.Database) (*river.Workers, error) {
	workers := river.NewWorkers()
	if err := river.AddWorkerSafely(workers, &RiverOpenFGAWorker{OfgaClient: ofgaConn, Database: *db}); err != nil {
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

// RiverConfig is a struct that represents the arguments to create the River instance.
type RiverConfig struct {
	// Config is an optional river config object that includes the retry policy,
	// the queue defaults and the maximum number of workers to create, as well as the workers themselves.
	Config *river.Config
	// Db holds a pointer to JIMM db.
	Db *db.Database
	// DbUrl is the DSN for JIMM db, which is used to create river's PG connection pool.
	DbUrl string
	// MaxAttempts is how many times river would retry before a job is abandoned and set as
	// discarded. This includes the original and the retry attempts.
	MaxAttempts int
	// MaxWorkers configures the maximum number of workers to create per task/job type.
	MaxWorkers int
	// OfgaClient holds a pointer to a valid OpenFGA client.
	OfgaClient *openfga.OFGAClient
}

// NewRiver returns a new river instance after applying the needed migrations to the database.
// It will open a postgres connections pool that would be closed in the Cleanup routine.
func NewRiver(ctx context.Context, riverConfig RiverConfig) (*River, error) {
	maxAttempts := max(1, riverConfig.MaxAttempts)
	workers, err := RegisterJimmWorkers(ctx, riverConfig.OfgaClient, riverConfig.Db)
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
	dbPool, err := pgxpool.New(ctx, riverConfig.DbUrl)
	if err != nil {
		return nil, err
	}
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

type WaitConfig struct {
	Duration time.Duration
}
