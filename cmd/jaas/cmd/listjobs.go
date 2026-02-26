// Copyright 2026 Canonical.

package cmd

import (
	"fmt"
	"strings"

	"github.com/juju/cmd/v3"
	"github.com/juju/gnuflag"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/jujuclient"

	"github.com/canonical/jimm/v3/pkg/api/params"
)

// stringSliceFlag is a custom flag type that allows multiple flag invocations.
type stringSliceFlag []string

func (s *stringSliceFlag) Set(value string) error {
	*s = append(*s, value)
	return nil
}

func (s *stringSliceFlag) String() string {
	if s == nil {
		return ""
	}
	return strings.Join(*s, ",")
}

const (
	listjobsCommandDoc = `
Displays information on long-running jobs.

The command supports filtering by job kind and status, and allows you to
limit the number of results returned (up to 10,000 jobs).

Valid job statuses are: running, successful, pending, failed, unknown
`
	listjobsCommandExample = `
    juju jobs
    juju jobs --format json
    juju jobs --count 500
    juju jobs --kind backup --kind restore
    juju jobs --status running --status pending
    juju jobs --count 1000 --status failed --kind backup
`
)

// NewListjobsCommand returns a command to list controller information.
func NewListJobsCommand() cmd.Command {
	cmd := &listjobsCommand{}
	cmd.SetClientStore(jujuclient.NewFileClientStore())

	return modelcmd.WrapBase(cmd)
}

// listjobsCommand shows controller information
// for all jobs known to JIMM.
type listjobsCommand struct {
	jaasCommandBase
	out      cmd.Output
	count    int
	kinds    stringSliceFlag
	statuses stringSliceFlag
}

func (c *listjobsCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "jobs",
		Purpose:  "Lists all jobs known to JIMM.",
		Doc:      listjobsCommandDoc,
		Examples: listjobsCommandExample,
		Aliases:  []string{"list-jobs"},
	})
}

// SetFlags implements Command.SetFlags.
func (c *listjobsCommand) SetFlags(f *gnuflag.FlagSet) {
	c.CommandBase.SetFlags(f)
	c.out.AddFlags(f, "yaml", map[string]cmd.Formatter{
		"yaml": cmd.FormatYaml,
		"json": cmd.FormatJson,
	})
	f.IntVar(&c.count, "count", 100, "Maximum number of jobs to return (max 10000)")
	f.Var(&c.kinds, "kind", "Filter jobs by kind (can be specified multiple times)")
	f.Var(&c.statuses, "status", "Filter jobs by status (can be specified multiple times)")
}

// Run implements Command.Run.
func (c *listjobsCommand) Run(ctxt *cmd.Context) error {
	if c.count <= 0 {
		return fmt.Errorf("count must be greater than 0")
	}
	if c.count > 10000 {
		return fmt.Errorf("count cannot exceed 10000, got %d", c.count)
	}

	client, err := c.getJIMMAPI()
	if err != nil {
		return err
	}
	defer client.Close()

	// Validate and convert statuses
	var statuses []params.JobStatus
	if len(c.statuses) > 0 {
		validStatuses := map[string]params.JobStatus{
			"running":    params.StatusRunning,
			"successful": params.StatusSuccessful,
			"pending":    params.StatusPending,
			"failed":     params.StatusFailed,
			"unknown":    params.StatusUnknown,
		}

		for _, s := range c.statuses {
			s = strings.TrimSpace(s)
			status, ok := validStatuses[s]
			if !ok {
				return fmt.Errorf("invalid status %q, must be one of: running, successful, pending, failed, unknown", s)
			}
			statuses = append(statuses, status)
		}
	}

	// Trim kinds
	kinds := make([]string, len(c.kinds))
	for i, k := range c.kinds {
		kinds[i] = strings.TrimSpace(k)
	}

	resp, err := client.ListJobs(&params.ListJobsRequest{
		Count:    c.count,
		Kinds:    kinds,
		Statuses: statuses,
	})
	if err != nil {
		return err
	}

	err = c.out.Write(ctxt, resp.Jobs)
	if err != nil {
		return err
	}

	return nil
}
