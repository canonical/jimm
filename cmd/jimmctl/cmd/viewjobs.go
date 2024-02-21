// Copyright 2024 Canonical Ltd.

package cmd

import (
	"fmt"
	"io"

	"github.com/gosuri/uitable"
	"github.com/juju/cmd/v3"
	"github.com/juju/gnuflag"
	jujuapi "github.com/juju/juju/api"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/jujuclient"
	"github.com/riverqueue/river/rivertype"

	"github.com/canonical/jimm/api"
	apiparams "github.com/canonical/jimm/api/params"
	"github.com/canonical/jimm/internal/errors"
)

const viewJobsDoc = `
	view-jobs allows you to view failed/canceled/completed jobs from the DB.

	Examples:
		jimmctl view-jobs 
		jimmctl view-jobs --limit 100 --sort-ascending
		jimmctl view-jobs --view-completed --format json
`

func NewViewJobsCommand() cmd.Command {
	cmd := &viewJobsCommand{
		store: jujuclient.NewFileClientStore(),
	}
	return modelcmd.WrapBase(cmd)
}

type viewJobsCommand struct {
	modelcmd.ControllerCommandBase
	out cmd.Output

	store    jujuclient.ClientStore
	dialOpts *jujuapi.DialOpts
	args     apiparams.ViewJobsRequest
}

// Info implements Command.Info. It returns the command information.
func (c *viewJobsCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "view-jobs",
		Purpose: "Interact with jimm's job engine to see jobs, their statistics, and arguments",
		Doc:     viewJobsDoc,
	})
}

// Init implements Command.Init. It checks the number of arguments and validates
// the date.
func (c *viewJobsCommand) Init(args []string) error {
	return nil
}

// Run implements Command.Run.
func (c *viewJobsCommand) Run(ctx *cmd.Context) error {
	currentController, err := c.store.CurrentController()
	if err != nil {
		return errors.E(err, "could not determine controller")
	}

	apiCaller, err := c.NewAPIRootWithDialOpts(c.store, currentController, "", c.dialOpts)
	if err != nil {
		return err
	}

	client := api.NewClient(apiCaller)
	jobs, err := client.ViewJobs(&c.args)
	if err != nil {
		return errors.E(err)
	}

	err = c.out.Write(ctx, jobs)
	if err != nil {
		return errors.E(err)
	}
	return nil
}

func formatJobsTabular(writer io.Writer, value interface{}) error {
	jobs, ok := value.(apiparams.RiverJobs)
	if !ok {
		return errors.E(fmt.Sprintf("expected value of type %T, got %T", jobs, value))
	}

	table := uitable.New()
	table.MaxColWidth = 50
	table.Wrap = true

	printJobs := func(jobsList []rivertype.JobRow, state rivertype.JobState) {
		table.AddRow(state, "ID", "Attempt", "Attempted At", "Created At", "Kind", "Finalized At", "Args", "Errors")
		for _, job := range jobsList {
			table.AddRow(
				"",
				job.ID,
				job.Attempt,
				job.AttemptedAt,
				job.CreatedAt,
				job.Kind,
				job.FinalizedAt,
				string(job.EncodedArgs[:]),
				job.Errors)
		}
		table.AddRow()
	}
	printJobs(jobs.CompletedJobs, rivertype.JobStateCompleted)
	printJobs(jobs.CancelledJobs, rivertype.JobStateCancelled)
	printJobs(jobs.FailedJobs, rivertype.JobStateDiscarded)

	fmt.Fprint(writer, table)
	return nil
}

// SetFlags implements Command.SetFlags.
func (c *viewJobsCommand) SetFlags(f *gnuflag.FlagSet) {
	c.CommandBase.SetFlags(f)
	c.out.AddFlags(f, "yaml", map[string]cmd.Formatter{
		"yaml":    cmd.FormatYaml,
		"json":    cmd.FormatJson,
		"tabular": formatJobsTabular,
	})
	f.IntVar(&c.args.Limit, "limit", 100, "limit the maximum number of returned jobs per state.")
	f.BoolVar(&c.args.SortAsc, "sort-ascending", false, "return the jobs from the oldest to the newest")
	f.BoolVar(&c.args.IncludeFailed, "view-failed", true, "return jobs that were discarded")
	f.BoolVar(&c.args.IncludeCancelled, "view-cancelled", true, "return jobs that were cancelled")
	f.BoolVar(&c.args.IncludeCompleted, "view-completed", false, "return jobs that completed successfully")
}
