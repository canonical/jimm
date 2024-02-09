// Copyright 2024 Canonical Ltd.

package cmd

import (
	"fmt"
	"io"
	"strings"

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

const jobViewerDoc = `
	job-viewer allows you to view failed/canceled/completed jobs from the DB.

	Examples:
		jimmctl job-viewer 
		jimmctl job-viewer --limit 100 --reverse
		jimmctl job-viewer --getCompleted --format json
`

func NewJobViewerCommand() cmd.Command {
	cmd := &jobViewerCommand{
		store: jujuclient.NewFileClientStore(),
	}
	return modelcmd.WrapBase(cmd)
}

type jobViewerCommand struct {
	modelcmd.ControllerCommandBase
	out cmd.Output

	store    jujuclient.ClientStore
	dialOpts *jujuapi.DialOpts
	args     apiparams.ViewJobsRequest
}

// Info implements Command.Info. It returns the command information.
func (c *jobViewerCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "job-viewer",
		Purpose: "Interact with jimm's job engine to see jobs, their statistics, and arguments.",
		Doc:     jobViewerDoc,
	})
}

// Init implements Command.Init. It checks the number of arguments and validates
// the date.
func (c *jobViewerCommand) Init(args []string) error {
	return nil
}

// Run implements Command.Run.
func (c *jobViewerCommand) Run(ctx *cmd.Context) error {
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
func (c *jobViewerCommand) SetFlags(f *gnuflag.FlagSet) {
	c.CommandBase.SetFlags(f)
	c.out.AddFlags(f, "yaml", map[string]cmd.Formatter{
		"yaml":    cmd.FormatYaml,
		"json":    cmd.FormatJson,
		"tabular": formatJobsTabular,
	})
	var JobKinds string
	// UNUSED FOR NOW
	f.StringVar(&c.args.After, "after", "", "display events that happened after specified time")
	f.StringVar(&c.args.Before, "before", "", "display events that happened before specified time")
	f.StringVar(&JobKinds, "jobKinds", "", "display events for specific jobKinds. use comma-separated workers, leave unspecified for all jobKinds")
	f.IntVar(&c.args.Offset, "offset", 0, "pagination offset")
	c.args.JobKind = strings.Split(JobKinds, ",")
	// USED
	f.IntVar(&c.args.Limit, "limit", 100, "limit the maximum number of returned jobs per state.")
	f.BoolVar(&c.args.SortAsc, "sortAsc", false, "return the jobs from the oldest to the newest")
	f.BoolVar(&c.args.GetFailed, "getFailed", true, "return jobs that were discarded")
	f.BoolVar(&c.args.GetCancelled, "getCanceled", true, "return jobs that were cancelled")
	f.BoolVar(&c.args.GetCompleted, "getCompleted", false, "return jobs that completed successfully")
}
