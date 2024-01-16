// Copyright 2024 Canonical Ltd.

package jimm

import (
	"context"
	"log/slog"

	"github.com/canonical/jimm/internal/dbmodel"
	"github.com/canonical/jimm/internal/openfga"
	ofganames "github.com/canonical/jimm/internal/openfga/names"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v4"
	"github.com/juju/zaputil/zapctx"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivermigrate"
	"go.uber.org/zap"
)

type OpenFGAArgs struct {
	client     openfga.OFGAClient
	controller *dbmodel.Controller
	model      *dbmodel.Model
	owner      *dbmodel.User
	modelInfo  *jujuparams.ModelInfo
}

func (OpenFGAArgs) Kind() string { return "OpenFGA" }

type OpenFGAWorker struct {
	river.WorkerDefaults[OpenFGAArgs]
}

func (w *OpenFGAWorker) Work(ctx context.Context, job *river.Job[OpenFGAArgs]) error {
	model := job.Args.model
	controller := job.Args.controller
	owner := job.Args.owner
	modelInfo := job.Args.modelInfo
	if err := job.Args.client.AddControllerModel(
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
	err := openfga.NewUser(owner, &job.Args.client).SetModelAccess(ctx, names.NewModelTag(modelInfo.UUID), ofganames.AdministratorRelation)
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

func RegisterJimmWorkers(ctx context.Context) *river.Workers {
	workers := river.NewWorkers()
	if err := river.AddWorkerSafely(workers, &OpenFGAWorker{}); err != nil {
		zapctx.Error(ctx, "Failed to register OpenFGA client", zap.Error(err))
	}
	return workers
}

type River struct {
	Client *river.Client[pgx.Tx]
}

func doMigration(ctx context.Context, dburl string) {
	var dbPool *pgxpool.Pool
	migrator := rivermigrate.New(riverpgxv5.New(dbPool), nil)
	dbPool, err := pgxpool.New(ctx, dburl)
	if err != nil {
		panic(err)
	}
	defer dbPool.Close()

	tx, err := dbPool.Begin(ctx)
	if err != nil {
		panic(err)
	}
	defer tx.Rollback(ctx)
	_, err = migrator.MigrateTx(ctx, tx, rivermigrate.DirectionUp, nil)
	if err != nil {
		zapctx.Error(ctx, "Failed to apply DB migration", zap.Error(err))
	}
}

func NewRiver(config *river.Config, db_url string, ctx context.Context) *River {
	doMigration(ctx, db_url)
	if config == nil {
		config = &river.Config{
			RetryPolicy: &river.DefaultClientRetryPolicy{},
			Queues: map[string]river.QueueConfig{
				river.QueueDefault: {MaxWorkers: 100},
			},
			Logger:  slog.Default(),
			Workers: RegisterJimmWorkers(ctx),
		}
	}
	dbPool, err := pgxpool.New(ctx, db_url)
	if err != nil {
		zapctx.Error(ctx, "Failed to create db pool", zap.Error(err))
	}
	riverClient, err := river.NewClient(riverpgxv5.New(dbPool), config)
	if err != nil {
		zapctx.Error(ctx, "Failed to create river client", zap.Error(err))
	}
	if err := riverClient.Start(ctx); err != nil {
		zapctx.Error(ctx, "FailedFailed to start river client", zap.Error(err))
	}
	r := River{
		Client: riverClient,
	}
	return &r
}
