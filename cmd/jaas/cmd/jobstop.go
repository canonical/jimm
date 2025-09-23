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
	jobStopCommandDoc = `
Stop a job.
`
	jobStopCommandExample = `
    juju job-stop 2cb433a6-04eb-4ec4-9567-90426d20a004 
`
)

// NewJobStopCommand returns a command to stop a job.
func NewJobStopCommand() cmd.Command {
	cmd := &jobStopCommand{
		store: jujuclient.NewFileClientStore(),
	}
	cmd.jobAPIFunc = cmd.newClient

	return modelcmd.WrapBase(cmd)
}

// jobStopCommand to stop a job.
type jobStopCommand struct {
	modelcmd.ControllerCommandBase

	store      jujuclient.ClientStore
	dialOpts   *jujuapi.DialOpts
	jobId      string
	jobAPIFunc func() (JIMMAPI, error)
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
		return errors.E("missing job id")
	}
	c.jobId, args = args[0], args[1:]
	if len(args) > 0 {
		return errors.E("unknown arguments")
	}

	return nil
}

// Run implements cmd.Command.Run interface.
func (c *jobStopCommand) Run(ctxt *cmd.Context) error {
	client, err := c.jobAPIFunc()
	if err != nil {
		return fmt.Errorf("failed to create JIMM client: %v", err)
	}

	err = client.StopJob(&params.StopJobRequest{
		JobID: c.jobId,
	})
	if err != nil {
		return fmt.Errorf("failed to stop job: %v", err)
	}
	_, err = ctxt.Stdout.Write([]byte(fmt.Sprintf("Job %s has been stopped.\n", c.jobId)))
	if err != nil {
		return fmt.Errorf("failed to write output: %v", err)
	}
	return nil
}

func (s *jobStopCommand) newClient() (JIMMAPI, error) {
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
