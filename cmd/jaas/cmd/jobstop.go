// Copyright 2026 Canonical.

package cmd

import (
	"fmt"

	"github.com/juju/cmd/v3"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/jujuclient"

	"github.com/canonical/jimm/v3/pkg/api/params"
)

const (
	jobStopCommandDoc = `
Stop a job.
`
	jobStopCommandExample = `
    juju job-stop 2cb433a6-04eb-4ec4-9567-90426d20a004
`
)

// NewJobStopCommand returns a command to stop a job.
func NewJobStopCommand() cmd.Command {
	cmd := &jobStopCommand{}
	cmd.SetClientStore(jujuclient.NewFileClientStore())

	return modelcmd.WrapBase(cmd)
}

// jobStopCommand to stop a job.
type jobStopCommand struct {
	jaasCommandBase

	jobId string
}

// Info implements cmd.Info interface.
func (c *jobStopCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "job-stop",
		Args:     "<job uuid>",
		Purpose:  "Stop a job",
		Doc:      jobStopCommandDoc,
		Examples: jobStopCommandExample,
	})
}

// Init implements the cmd.Command interface.
func (c *jobStopCommand) Init(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("missing job id")
	}
	c.jobId, args = args[0], args[1:]
	if len(args) > 0 {
		return fmt.Errorf("unknown arguments")
	}

	return nil
}

// Run implements cmd.Command.Run interface.
func (c *jobStopCommand) Run(ctxt *cmd.Context) error {
	client, err := c.getJIMMAPI()
	if err != nil {
		return fmt.Errorf("failed to create JIMM client: %v", err)
	}

	err = client.StopJob(&params.StopJobRequest{
		JobID: c.jobId,
	})
	if err != nil {
		return fmt.Errorf("failed to stop job: %v", err)
	}
	_, err = fmt.Fprintf(ctxt.Stdout, "Job %s has been stopped.\n", c.jobId)
	if err != nil {
		return fmt.Errorf("failed to write output: %v", err)
	}
	return nil
}
