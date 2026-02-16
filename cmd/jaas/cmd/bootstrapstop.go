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
Stop a bootstrap job.
`
	jobStopCommandExample = `
    juju bootstrap-stop <id>
`
)

// NewBootstrapStopCommand returns a command to stop an in-progress bootstrap job.
func NewBootstrapStopCommand() cmd.Command {
	cmd := &bootstrapStopCommand{}
	cmd.SetClientStore(jujuclient.NewFileClientStore())

	return modelcmd.WrapBase(cmd)
}

// bootstrapStopCommand to stop a job.
type bootstrapStopCommand struct {
	jaasCommandBase

	jobId string
}

// Info implements cmd.Info interface.
func (c *bootstrapStopCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "bootstrap-stop",
		Args:     "<job id>",
		Purpose:  "Stop an in-progress bootstrap job",
		Doc:      jobStopCommandDoc,
		Examples: jobStopCommandExample,
	})
}

// Init implements the cmd.Command interface.
func (c *bootstrapStopCommand) Init(args []string) error {
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
func (c *bootstrapStopCommand) Run(ctxt *cmd.Context) error {
	client, err := c.getJIMMAPI()
	if err != nil {
		return fmt.Errorf("failed to create JIMM client: %v", err)
	}

	err = client.StopBootstrap(&params.StopBootstrapRequest{
		JobID: c.jobId,
	})
	if err != nil {
		return fmt.Errorf("failed to stop bootstrap: %v", err)
	}
	_, err = fmt.Fprintf(ctxt.Stdout, "Bootstrap job with ID %q has been stopped.\n", c.jobId)
	if err != nil {
		return fmt.Errorf("failed to write output: %v", err)
	}
	return nil
}
