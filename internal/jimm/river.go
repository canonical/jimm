// Copyright 2024 Canonical Ltd.

package jimm

import (
	"context"
	"log/slog"

	"github.com/canonical/jimm/internal/db"
	"github.com/canonical/jimm/internal/dbmodel"
	"github.com/canonical/jimm/internal/openfga"
	ofganames "github.com/canonical/jimm/internal/openfga/names"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/juju/names/v4"
	"github.com/juju/zaputil/zapctx"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivermigrate"
	"go.uber.org/zap"
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
	OfgaClient openfga.OFGAClient
	Database   db.Database
}

// Work is tha function executed by the worker when it picks up the job.
func (w *OpenFGAWorker) Work(ctx context.Context, job *river.Job[OpenFGAArgs]) error {

	controller := &dbmodel.Controller{UUID: job.Args.ControllerUUID}
	err := w.Database.GetController(ctx, controller)
	if err != nil {
		return err
	}

	model := &dbmodel.Model{ID: job.Args.ModelId}
	err = w.Database.GetModel(ctx, model)
	if err != nil {
		return err
	}

	owner := &dbmodel.User{Username: job.Args.OwnerName}
	err = w.Database.GetUser(ctx, owner)
	if err != nil {
		return err
	}

	if err := w.OfgaClient.AddControllerModel(
		ctx,
		controller.ResourceTag(),
		model.ResourceTag(),
	); err != nil {
		zapctx.Error(
			ctx,
			"failed to add controller-model relation",
			zap.String("controller", controller.UUID),
			zap.String("model", model.UUID.String),
		)
		return err
	}
	err = openfga.NewUser(owner, &w.OfgaClient).SetModelAccess(ctx, names.NewModelTag(job.Args.ModelInfoUUID), ofganames.AdministratorRelation)
	if err != nil {
		zapctx.Error(
			ctx,
			"failed to add administrator relation",
			zap.String("user", owner.Tag().String()),
			zap.String("model", model.UUID.String),
		)
		return err
	}
	return nil
}

func RegisterJimmWorkers(ctx context.Context, ofgaConn openfga.OFGAClient, db db.Database) (*river.Workers, error) {
	workers := river.NewWorkers()
	if err := river.AddWorkerSafely(workers, &OpenFGAWorker{OfgaClient: ofgaConn, Database: db}); err != nil {
		return nil, err
	}
	return workers, nil
}

// River is the struct that holds that Client connection to river.
type River struct {
	Client *river.Client[pgx.Tx]
}

func doMigration(ctx context.Context, dburl string) error {
	var dbPool *pgxpool.Pool
	migrator := rivermigrate.New(riverpgxv5.New(dbPool), nil)
	dbPool, err := pgxpool.New(ctx, dburl)
	if err != nil {
		return err
	}
	defer dbPool.Close()

	tx, err := dbPool.Begin(ctx)
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

func NewRiver(config *river.Config, db_url string, ctx context.Context, ofgaConn openfga.OFGAClient, db db.Database) (*River, error) {
	err := doMigration(ctx, db_url)
	if err != nil {
		return nil, err
	}

	if config == nil {
		workers, err := RegisterJimmWorkers(ctx, ofgaConn, db)
		if err != nil {
			return nil, err
		}
		config = &river.Config{
			RetryPolicy: &river.DefaultClientRetryPolicy{},
			Queues: map[string]river.QueueConfig{
				river.QueueDefault: {MaxWorkers: 100},
			},
			Logger:  slog.Default(),
			Workers: workers,
		}
	}
	dbPool, err := pgxpool.New(ctx, db_url)
	if err != nil {
		return nil, err
	}
	riverClient, err := river.NewClient(riverpgxv5.New(dbPool), config)
	if err != nil {
		return nil, err
	}
	if err := riverClient.Start(ctx); err != nil {
		return nil, err
	}
	r := River{
		Client: riverClient,
	}
	return &r, nil
}
