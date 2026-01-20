package river

import (
	"context"
	"database/sql"

	"github.com/canonical/jimm/v3/internal/db"
	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/openfga"
	"github.com/juju/version/v2"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverdatabasesql"
	"github.com/riverqueue/river/rivermigrate"
)

// UpgradeManager defines the methods required from this manager for the workers.
type UpgradeManager interface {
	// MigrateModel migrates a model to a new controller without upgrading the model's agent.
	MigrateModel(ctx context.Context, user *openfga.User, modelUUID string, targetControllerName string) error
	// UpgradeModel upgrades a model to the target version.
	UpgradeModel(ctx context.Context, modelUUID string, targetVersion version.Number) error
}

// Store defines a method to retrieve a user from the database for the purpose
// of authenticating river jobs.
type Store interface {
	FetchIdentity(ctx context.Context, u *dbmodel.Identity) (err error)
}

// StartWorkers sets up and starts the river workers.
// Start() is a non-blocking call; it starts a background goroutine to process jobs, and maintainance tasks.
func StartWorkers(
	ctx context.Context,
	db *db.Database,
	openfgaClient *openfga.OFGAClient,
	upgradeManager UpgradeManager,
) error {
	workers := river.NewWorkers()

	migrationWorker, err := newMigrationWorker(openfgaClient, db, upgradeManager)
	if err != nil {
		return err
	}
	err = river.AddWorkerSafely(workers, migrationWorker)
	if err != nil {
		return err
	}

	upgradeWorker, err := newUpgradeWorker(upgradeManager)
	if err != nil {
		return err
	}
	err = river.AddWorkerSafely(workers, upgradeWorker)
	if err != nil {
		return err
	}

	sqlDb, err := db.SqlDB()
	if err != nil {
		return err
	}
	riverClient, err := river.NewClient(riverdatabasesql.New(sqlDb), &river.Config{
		Queues: map[string]river.QueueConfig{
			river.QueueDefault: {MaxWorkers: 5},
		},
		Workers: workers,
	})
	if err != nil {
		return err
	}
	return riverClient.Start(ctx)
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

// NewRiverClient creates a new RiverClient instance.
func NewRiverClient(db *db.Database, cfg river.Config) (*river.Client[*sql.Tx], error) {
	sqlDb, err := db.SqlDB()
	if err != nil {
		return nil, err
	}
	client, err := river.NewClient(riverdatabasesql.New(sqlDb), &cfg)
	if err != nil {
		return nil, err
	}
	return client, nil
}
