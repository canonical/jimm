// Copyright 2026 Canonical.

package river

import (
	"context"
	"database/sql"
	"time"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverdatabasesql"
	"github.com/riverqueue/river/rivertype"

	"github.com/canonical/jimm/v3/internal/db"
	"github.com/canonical/jimm/v3/internal/rivertypes"
)

// Client wraps a River client and provides higher-level enqueue helpers.
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
func (c *Client) EnqueueUpgradeTo(ctx context.Context, args rivertypes.UpgradeToArgs) (*rivertype.JobInsertResult, error) {
	job, err := c.client.Insert(ctx, args, nil)
	return job, err
}

// EnqueueBootstrap inserts a River job to bootstrap a new controller.
func (c *Client) EnqueueBootstrap(ctx context.Context, args rivertypes.BootstrapArgs) (*rivertype.JobInsertResult, error) {
	job, err := c.client.Insert(ctx, args, nil)
	return job, err
}

// EnqueueDestroyController inserts a River job to destroy an existing controller.
func (c *Client) EnqueueDestroyController(ctx context.Context, args rivertypes.DestroyControllerArgs) (*rivertype.JobInsertResult, error) {
	job, err := c.client.Insert(ctx, args, nil)
	return job, err
}

// GetJobInfo returns the current state of the specified job.
func (c *Client) GetJobInfo(ctx context.Context, jobID int64) (*rivertype.JobRow, error) {
	return c.client.JobGet(ctx, jobID)
}

// CancelJob cancels the specified job. It returns the final job state after cancellation.
func (c *Client) CancelJob(ctx context.Context, jobID int64) (*rivertype.JobRow, error) {
	return c.client.JobCancel(ctx, jobID)
}

// WaitForJobCompletion polls the database, waiting for the specified job to complete,
// returning the final job state. If the job has already completed, it returns immediately.
func (c *Client) WaitForJobCompletion(ctx context.Context, jobID int64) (*rivertype.JobRow, error) {
	// River event subscriptions only emit events for jobs worked by the *same*
	// client instance. Callers waiting on a job that may be worked by another
	// client/process must poll for job state instead.
	const pollInterval = 500 * time.Millisecond

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		job, err := c.client.JobGet(ctx, jobID)
		if err != nil {
			return nil, err
		}
		if job.FinalizedAt != nil {
			return job, nil
		}

		select {
		case <-ticker.C:
			continue
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}
