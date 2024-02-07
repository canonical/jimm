// Copyright 2024 Canonical Ltd.

package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/canonical/jimm/internal/errors"
	"github.com/canonical/jimm/internal/jimm"
	"github.com/jackc/pgx/v5"
	"github.com/juju/cmd/v3"
	"github.com/juju/gnuflag"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
)

func NewRiverViewerCommand() cmd.Command {
	cmd := &riverViewerCommand{}
	return modelcmd.WrapBase(cmd)
}

type RiverViewerArgs struct {
	Reverse      bool
	After        string
	Before       string
	Workers      string
	Limit        int
	Offset       int
	GetFailed    bool
	GetCanceled  bool
	GetCompleted bool
}

type riverViewerCommand struct {
	modelcmd.ControllerCommandBase
	out    cmd.Output
	client *river.Client[pgx.Tx]
	args   RiverViewerArgs
}

// Info implements Command.Info. It returns the command information.
func (c *riverViewerCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "river-viewer",
		Args:    "<ISO8601 date>",
		Purpose: "purges audit logs from the database before the given date",
		Doc:     purgeLogsDoc,
	})
}

// Init implements Command.Init. It checks the number of arguments and validates
// the date.
func (c *riverViewerCommand) Init(args []string) error {
	return nil
}

func (c *riverViewerCommand) getJobs(ctx context.Context, state rivertype.JobState, jobsMap map[rivertype.JobState][]*rivertype.JobRow) error {
	sortOrder := river.SortOrderAsc
	if c.args.Reverse {
		sortOrder = river.SortOrderDesc
	}
	jobs, err := c.client.JobList(
		ctx,
		river.NewJobListParams().
			State(state).
			OrderBy(river.JobListOrderByTime, sortOrder).
			First(c.args.Limit),
	)
	if err != nil {
		return errors.E(fmt.Sprintf("failed to read %s jobs from river db, err: %s", state, err))
	}
	jobsMap[state] = jobs
	return nil
}

// Run implements Command.Run.
func (c *riverViewerCommand) Run(ctx *cmd.Context) error {
	DSN := os.Getenv("JIMM_DSN")
	if DSN == "" {
		return errors.E("failed to read JIMM_DSN from the environment")
	}
	r, err := jimm.NewRiver(ctx, jimm.RiverConfig{DSN: DSN, MaxAttempts: 3}, nil, nil, nil)
	if err != nil {
		return err
	}
	c.client = r.Client
	jobsMap := make(map[rivertype.JobState][]*rivertype.JobRow)
	if c.args.GetFailed {
		if err := c.getJobs(ctx, rivertype.JobStateDiscarded, jobsMap); err != nil {
			return err
		}
	}
	if c.args.GetCanceled {
		if err := c.getJobs(ctx, rivertype.JobStateCancelled, jobsMap); err != nil {
			return err
		}
	}
	if c.args.GetCompleted {
		if err := c.getJobs(ctx, rivertype.JobStateCompleted, jobsMap); err != nil {
			return err
		}
	}

	if err := c.out.Write(ctx, jobsMap); err != nil {
		return errors.E(fmt.Sprintf("failed to write output. err: %s", err))
	}

	if err := c.client.Stop(ctx); err != nil {
		return errors.E(fmt.Sprintf("failed to stop river client, err: %s", err))
	}
	return nil
}

// SetFlags implements Command.SetFlags.
func (c *riverViewerCommand) SetFlags(f *gnuflag.FlagSet) {
	c.CommandBase.SetFlags(f)
	c.out.AddFlags(f, "yaml", map[string]cmd.Formatter{
		"yaml":    cmd.FormatYaml,
		"json":    cmd.FormatJson,
		"tabular": formatTabular,
	})

	// UNUSED FOR NOW
	f.StringVar(&c.args.After, "after", "", "display events that happened after specified time")
	f.StringVar(&c.args.Before, "before", "", "display events that happened before specified time")
	f.StringVar(&c.args.Workers, "workers", "", "display events for specific workers. use comma-separated workers")
	f.IntVar(&c.args.Offset, "offset", 0, "pagination offset")

	// USED
	f.IntVar(&c.args.Limit, "limit", 100, "limit the maximum number of returned jobs per state.")
	f.BoolVar(&c.args.Reverse, "reverse", false, "reverse the order of jobs, showing the most recent first")
	f.BoolVar(&c.args.GetFailed, "getFailed", true, "return jobs that were discarded")
	f.BoolVar(&c.args.GetCanceled, "getCanceled", true, "return jobs that were cancelled")
	f.BoolVar(&c.args.GetCompleted, "getCompleted", false, "return jobs that completed successfully")

}
