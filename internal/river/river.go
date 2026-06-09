// Copyright 2026 Canonical.

package river

import (
	"context"
	"database/sql"

	"github.com/juju/version/v2"
	"github.com/juju/zaputil/zapctx"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverdatabasesql"
	"github.com/riverqueue/river/rivermigrate"
	"github.com/riverqueue/river/rivertype"
	"go.uber.org/zap"

	"github.com/canonical/jimm/v3/internal/db"
	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/jimm/bootstrap"
	_ "github.com/canonical/jimm/v3/internal/jimm/upgrade" // Dummy import to prevent future circular dependency
	"github.com/canonical/jimm/v3/internal/openfga"
)

const (
	defaultQueueMaxWorkers = 5
)

// UpgradeManager defines the methods for the domain logic of upgrades.
type UpgradeManager interface {
	// MigrateModel migrates a model to a new controller without upgrading the model's agent.
	MigrateModel(ctx context.Context, user *openfga.User, modelUUID string, targetControllerName string) error
	// UpgradeModel upgrades a model to the target version.
	UpgradeModel(ctx context.Context, modelUUID string, targetVersion version.Number) error
}

// BootstrapManager defines the methods for the domain logic of bootstrapping and destroying controllers.
type BootstrapManager interface {
	BootstrapController(ctx context.Context, p bootstrap.RunBootstrapArgs, cmdFactory bootstrap.CommandFactory, user *openfga.User) error
	DestroyController(ctx context.Context, p bootstrap.RunDestroyControllerArgs, cmdFactory bootstrap.CommandFactory, user *openfga.User) error
}

// Store defines a method to retrieve a user from the database for the purpose
// of authenticating river jobs.
type Store interface {
	FetchIdentity(ctx context.Context, u *dbmodel.Identity) (err error)
}

// StartWorkers sets up and starts the river workers.
// Start() is a non-blocking call; it starts a background goroutine to process jobs, and maintainance tasks.
// The started River client is returned so callers can wait for shutdown to complete.
func StartWorkers(
	ctx context.Context,
	db *db.Database,
	openfgaClient *openfga.OFGAClient,
	upgradeManager UpgradeManager,
	bootstrapManager BootstrapManager,
) (*river.Client[*sql.Tx], error) {
	workerParams := workerParams{
		openfgaClient:    openfgaClient,
		store:            db,
		upgradeManager:   upgradeManager,
		bootstrapManager: bootstrapManager,
	}
	workers, err := newWorkers(workerParams)
	if err != nil {
		return nil, err
	}

	sqlDb, err := db.SqlDB()
	if err != nil {
		return nil, err
	}

	riverClient, err := river.NewClient(riverdatabasesql.New(sqlDb), &river.Config{
		Queues: map[string]river.QueueConfig{
			river.QueueDefault: {MaxWorkers: defaultQueueMaxWorkers},
		},
		Workers:      workers,
		ErrorHandler: &errorHandler{},
	})
	if err != nil {
		return nil, err
	}
	if err := riverClient.Start(ctx); err != nil {
		return nil, err
	}
	return riverClient, nil
}

type workerParams struct {
	openfgaClient    *openfga.OFGAClient
	store            *db.Database
	upgradeManager   UpgradeManager
	bootstrapManager BootstrapManager
	upgradeToRetry   upgradeToRetryFunc
}

func newWorkers(wp workerParams) (*river.Workers, error) {
	workers := river.NewWorkers()

	upgradeToWorker, err := newUpgradeToWorker(wp.openfgaClient, wp.store, wp.upgradeManager, wp.upgradeToRetry)
	if err != nil {
		return nil, err
	}
	if err := river.AddWorkerSafely(workers, upgradeToWorker); err != nil {
		return nil, err
	}

	bootstrapWorker, err := newBootstrapWorker(wp.openfgaClient, wp.store, wp.bootstrapManager)
	if err != nil {
		return nil, err
	}
	if err := river.AddWorkerSafely(workers, bootstrapWorker); err != nil {
		return nil, err
	}

	destroyControllerWorker, err := newDestroyControllerWorker(wp.openfgaClient, wp.store, wp.bootstrapManager)
	if err != nil {
		return nil, err
	}
	if err := river.AddWorkerSafely(workers, destroyControllerWorker); err != nil {
		return nil, err
	}

	return workers, nil
}

// MigrateRiver performs the necessary migrations for the River job queue.
func MigrateRiver(ctx context.Context, db *db.Database) error {
	sqlDb, err := db.SqlDB()
	if err != nil {
		return err
	}
	migrator, err := rivermigrate.New(riverdatabasesql.New(sqlDb), nil)
	if err != nil {
		return err
	}
	_, err = migrator.Migrate(ctx, rivermigrate.DirectionUp, nil)
	if err != nil {
		return err
	}
	return nil
}

type errorHandler struct{}

// HandleError implements the [river.ErrorHandler] interface.
// We use this to log errors that occur in River jobs.
func (h *errorHandler) HandleError(ctx context.Context, job *rivertype.JobRow, err error) *river.ErrorHandlerResult {
	zapctx.Error(ctx, "river job encountered an error",
		zap.Int("attempt", job.Attempt),
		zap.String("job_kind", job.Kind),
		zap.Int64("job_id", job.ID),
		zap.Error(err))
	// No custom behavior; use default retry logic.
	return &river.ErrorHandlerResult{}
}

// HandlePanic implements the [river.ErrorHandler] interface.
// We use this to log panics that occur in River jobs.
func (h *errorHandler) HandlePanic(ctx context.Context, job *rivertype.JobRow, panicVal any, trace string) *river.ErrorHandlerResult {
	zapctx.Error(ctx, "river job encountered a panic",
		zap.Int("attempt", job.Attempt),
		zap.String("job_kind", job.Kind),
		zap.Int64("job_id", job.ID),
		zap.Any("panic value", panicVal),
		zap.String("trace", trace))
	// No custom behavior; use default retry logic.
	return &river.ErrorHandlerResult{}
}
