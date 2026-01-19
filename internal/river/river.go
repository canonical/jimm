package river

import (
	"context"
	"database/sql"

	"github.com/canonical/jimm/v3/internal/db"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverdatabasesql"
	"github.com/riverqueue/river/rivermigrate"
)

// StartWorkers sets up and starts the river workers.
// Start() is a non-blocking call; it starts a background goroutine to process jobs, and maintainance tasks.
func StartWorkers(ctx context.Context, db *db.Database, upgradeToManager UpgradeToManager) error {
	workers := river.NewWorkers()
	w, err := newUpgradeToWorker(upgradeToManager)
	if err != nil {
		return err
	}
	err = river.AddWorkerSafely(workers, w)
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
