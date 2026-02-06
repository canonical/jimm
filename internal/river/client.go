// Copyright 2026 Canonical.

package river

import (
	"context"
	"database/sql"

	"github.com/canonical/jimm/v3/internal/db"
	"github.com/canonical/jimm/v3/internal/rivertypes"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverdatabasesql"
	"github.com/riverqueue/river/rivertype"
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
func (c *Client) EnqueueUpgradeTo(ctx context.Context, args rivertypes.UpgradeToArgs) (int64, error) {
	job, err := c.client.Insert(ctx, args, nil)
	return job.Job.ID, err
}

// TODO(Kian JUJU-9159): Return the isDuplicate flag so we can either return an error to callers
// or at least inform them that a bootstrap is in-progress and their request was ignored.

// EnqueueBootstrap inserts a River job to bootstrap a new controller.
func (c *Client) EnqueueBootstrap(ctx context.Context, args rivertypes.BootstrapArgs) (int64, error) {
	job, err := c.client.Insert(ctx, args, nil)
	return job.Job.ID, err
}

// EnqueueDestroyController inserts a River job to destroy an existing controller.
func (c *Client) EnqueueDestroyController(ctx context.Context, args rivertypes.DestroyControllerArgs) (int64, error) {
	job, err := c.client.Insert(ctx, args, nil)
	return job.Job.ID, err
}

// GetJobInfo returns the current state of the specified job.
func (c *Client) GetJobInfo(ctx context.Context, jobID int64) (*rivertype.JobRow, error) {
	return c.client.JobGet(ctx, jobID)
}

// CancelJob cancels the specified job. It returns the final job state after cancellation.
func (c *Client) CancelJob(ctx context.Context, jobID int64) (*rivertype.JobRow, error) {
	return c.client.JobCancel(ctx, jobID)
}

// WaitForJobCompletion waits for the specified job to complete, returning the final job state.
// If the job has already completed, it returns immediately.
//
// Specify a context with an appropriate timeout when calling this method to avoid waiting indefinitely.
func (c *Client) WaitForJobCompletion(ctx context.Context, jobID int64) (*rivertype.JobRow, error) {
	// Subscribe to job completion events before checking the job status to avoid
	// missing the completion event in case the job completes between the JobGet and Subscribe calls.
	subscribeChan, subscribeCancel :=
		c.client.Subscribe(
			river.EventKindJobCompleted,
			river.EventKindJobCancelled,
			river.EventKindJobFailed,
		)
	defer subscribeCancel()

	job, err := c.client.JobGet(ctx, jobID)
	if err != nil {
		return nil, err
	}

	if job.FinalizedAt != nil {
		return job, nil
	}

	for {
		select {
		case event := <-subscribeChan:
			if event.Job.ID == jobID {
				return event.Job, nil
			}
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}
