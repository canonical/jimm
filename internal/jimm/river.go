// Copyright 2024 Canonical Ltd.

package jimm

import (
	"context"

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

// OpenFGAArgs holds the river job arguments for writing tuples to OpenFGA.
type OpenFGAArgs struct {
	ControllerUUID string `json:"controller_uuid"`
	ModelId        uint   `json:"model_id"`
	ModelInfoUUID  string `json:"model_info_uuid"`
	OwnerName      string `json:"owner_name"`
}

// Kind returns the kind of the job to be picked up by the appropriate river workers.
// It is the same concept as Kafka topics.
func (OpenFGAArgs) Kind() string { return "OpenFGA" }

// OpenFGAWorker is the river worker that would run the job.
type OpenFGAWorker struct {
	river.WorkerDefaults[OpenFGAArgs]
	OfgaClient *openfga.OFGAClient
	Database   db.Database
}

// Work is tha function executed by the worker when it picks up the job.
func (w *OpenFGAWorker) Work(ctx context.Context, job *river.Job[OpenFGAArgs]) error {
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
		return errors.E(err, "Failed to add the controller-model relation from the river job.")
	}
	err = openfga.NewUser(owner, w.OfgaClient).SetModelAccess(ctx, names.NewModelTag(job.Args.ModelInfoUUID), ofganames.AdministratorRelation)
	if err != nil {
		zapctx.Error(
			ctx,
			"failed to add administrator relation",
			zap.String("user", owner.Tag().String()),
			zap.String("model", modelTag.Id()),
		)
		return errors.E(err, "Failed to add the adminstrator relation from the river job.")
	}
	return nil
}

// River is the struct that holds that Client connection to river.
type River struct {
	Client      *river.Client[pgx.Tx]
	dbPool      *pgxpool.Pool
	MaxAttempts int
	RetryPolicy river.ClientRetryPolicy
}

// RegisterJimmWorkers would register known workers safely and return a pointer to a river.workers struct that should be used in river creation.
func RegisterJimmWorkers(ctx context.Context, ofgaConn *openfga.OFGAClient, db *db.Database) (*river.Workers, error) {
	workers := river.NewWorkers()
	if err := river.AddWorkerSafely(workers, &OpenFGAWorker{OfgaClient: ofgaConn, Database: *db}); err != nil {
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
		zapctx.Error(ctx, "Failed to apply DB migration", zap.Error(err))
		rollbackErr := tx.Rollback(ctx)
		if rollbackErr != nil {
			zapctx.Error(ctx, "Failed to rollback DB migration", zap.Error(rollbackErr))
		}
		return err
	}
	err = tx.Commit(ctx)
	if err != nil {
		zapctx.Error(ctx, "Failed to commit DB migration", zap.Error(err))
		return err
	}
	return nil
}

// NewRiverArgs is a struct that represents
type NewRiverArgs struct {
	Config      *river.Config
	Db          *db.Database
	DbUrl       string
	MaxAttempts int
	MaxWorkers  int
	OfgaClient  *openfga.OFGAClient
}

// NewRiver would return a new river instance, and it would apply the needed migrations to the database,
// and open a postgres connections pool that would be closed in the Cleanup routine.
func NewRiver(ctx context.Context, newRiverArgs NewRiverArgs) (*River, error) {
	maxAttempts := max(1, newRiverArgs.MaxAttempts)
	workers, err := RegisterJimmWorkers(ctx, newRiverArgs.OfgaClient, newRiverArgs.Db)
	if err != nil {
		return nil, err
	}
	if newRiverArgs.Config == nil {
		newRiverArgs.Config = &river.Config{
			RetryPolicy: &river.DefaultClientRetryPolicy{},
			Queues: map[string]river.QueueConfig{
				river.QueueDefault: {MaxWorkers: max(1, newRiverArgs.MaxWorkers)},
			},
			// Logger:  slog.Default(),
			Workers: workers,
		}
	}
	dbPool, err := pgxpool.New(ctx, newRiverArgs.DbUrl)
	if err != nil {
		return nil, err
	}
	riverClient, err := river.NewClient(riverpgxv5.New(dbPool), newRiverArgs.Config)
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
