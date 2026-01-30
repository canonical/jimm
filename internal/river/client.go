// Copyright 2026 Canonical.

package river

import (
	"context"
	"database/sql"

	"github.com/canonical/jimm/v3/internal/db"
	"github.com/canonical/jimm/v3/internal/rivertypes"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverdatabasesql"
)

// Client wraps a River client and provides higher-level enqueue helpers.
//
// In particular, it implements [ports.UpgradeEnqueuer] for enqueueing upgrade
// orchestration jobs.
type Client struct {
	client *river.Client[*sql.Tx]
}

// NewRiverClient creates a new Client instance.
func NewRiverClient(db *db.Database) (*Client, error) {
	sqlDb, err := db.SqlDB()
	if err != nil {
		return nil, err
	}
	client, err := river.NewClient(riverdatabasesql.New(sqlDb), &river.Config{})
	if err != nil {
		return nil, err
	}
	return &Client{client: client}, nil
}

// EnqueueUpgradeTo inserts a River job to upgrade a model to the specified version
// by migrating and upgrading it.
func (c *Client) EnqueueUpgradeTo(args rivertypes.UpgradeToArgs) (int64, error) {
	job, err := c.client.Insert(context.Background(), args, nil)
	return job.Job.ID, err
}
