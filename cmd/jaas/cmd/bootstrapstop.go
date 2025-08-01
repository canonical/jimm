// Copyright 2025 Canonical.

package cmd

import (
	"fmt"

	"github.com/juju/cmd/v3"
	jujuapi "github.com/juju/juju/api"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/jujuclient"

	"github.com/canonical/jimm/v3/internal/errors"
	jimmAPI "github.com/canonical/jimm/v3/pkg/api"
	"github.com/canonical/jimm/v3/pkg/api/params"
)

const (
	bootstrapStopCommandDoc = `
Stop a bootstrap job.
`
	bootstrapStopCommandExample = `
    juju bootstrap-stop 2cb433a6-04eb-4ec4-9567-90426d20a004 
`
)

// NewBootstrapStopCommand returns a command to stop a bootstrap job.
func NewBootstrapStopCommand() cmd.Command {
	cmd := &bootstrapStopCommand{
		store: jujuclient.NewFileClientStore(),
	}
	cmd.bootstrapAPIFunc = cmd.newClient

	return modelcmd.WrapBase(cmd)
}

// bootstrapStopCommand to stop a bootstrap job.
type bootstrapStopCommand struct {
	modelcmd.ControllerCommandBase

	store            jujuclient.ClientStore
	dialOpts         *jujuapi.DialOpts
	jobId            string
	bootstrapAPIFunc func() (JIMMAPI, error)
}

// Info implements cmd.Info interface.
func (c *bootstrapStopCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "bootstrap-stop",
		Args:     "<job uuid>",
		Purpose:  "Stop a bootstrap job",
		Doc:      bootstrapStopCommandDoc,
		Examples: bootstrapStopCommandExample,
	})
}

// Init implements the cmd.Command interface.
func (c *bootstrapStopCommand) Init(args []string) error {
	if len(args) < 1 {
		return errors.E("missing job id")
	}
	c.jobId, args = args[0], args[1:]
	if len(args) > 0 {
		return errors.E("unknown arguments")
	}

	return nil
}

// Run implements cmd.Command.Run interface.
func (c *bootstrapStopCommand) Run(ctxt *cmd.Context) error {
	client, err := c.bootstrapAPIFunc()
	if err != nil {
		return fmt.Errorf("failed to create JIMM client: %v", err)
	}

	err = client.BootstrapStop(&params.BootstrapStopRequest{
		JobID: c.jobId,
	})
	if err != nil {
		return fmt.Errorf("failed to stop bootstrap job: %v", err)
	}
	_, err = ctxt.Stdout.Write([]byte(fmt.Sprintf("Bootstrap job %s has been stopped.\n", c.jobId)))
	if err != nil {
		return fmt.Errorf("failed to write output: %v", err)
	}
	return nil
}

func (s *bootstrapStopCommand) newClient() (JIMMAPI, error) {
	currentController, err := s.store.CurrentController()
	if err != nil {
		return nil, errors.E(err, "could not determine controller")
	}

	apiCaller, err := s.NewAPIRootWithDialOpts(s.store, currentController, "", s.dialOpts)
	if err != nil {
		return nil, err
	}

	return jimmAPI.NewClient(apiCaller), nil
}
