// Copyright 2025 Canonical.

package cmd

import (
	"fmt"

	"github.com/juju/cmd/v3"
	"github.com/juju/gnuflag"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/jujuclient"

	jimmAPI "github.com/canonical/jimm/v3/pkg/api"
	"github.com/canonical/jimm/v3/pkg/api/params"
)

const (
	destroyControllerDoc = `

`
	destroyControllerExamples = `
	juju [jaas] destroy-controller <controller name>
	juju [jaas] bootstrap mycontroller
`
)

// destroyControllerCommand starts a bootstrap jobon the controller.
type destroyControllerCommand struct {
	modelcmd.ControllerCommandBase
	out cmd.Output

	store jujuclient.ClientStore

	cloud          string
	region         string
	controllerName string

	// Flags
	detach bool
}

// NewDestroyControllerStartCommand returns a command to start a job
// that will destroy a Juju controller.
func NewDestroyControllerStartCommand() cmd.Command {
	cmd := &destroyControllerCommand{
		store: jujuclient.NewFileClientStore(),
	}

	return modelcmd.WrapBase(cmd)
}

// Init implements modelcmd.Command.
func (c *destroyControllerCommand) Init(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("missing controller name")
	}

	c.controllerName, args = args[0], args[1:]
	if len(args) > 0 {
		return fmt.Errorf("unknown arguments")
	}
	return nil
}

// SetFlags implements modelcmd.Command.
func (c *destroyControllerCommand) SetFlags(f *gnuflag.FlagSet) {
	c.CommandBase.SetFlags(f)
	c.out.AddFlags(f, "json", map[string]cmd.Formatter{
		"yaml": cmd.FormatYaml,
		"json": cmd.FormatJson,
	})
	f.BoolVar(&c.detach, "detach", false, "If set, the command will start the bootstrap job and return immediately with the job ID, without waiting for the job to complete.")
}

// Info implements modelcmd.Command.
func (c *destroyControllerCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "destroy-controller",
		Args:     "<controller name>",
		Purpose:  "Destroy a Juju controller via JIMM",
		Doc:      destroyControllerDoc,
		Examples: destroyControllerExamples,
	})
}

// Run implements modelcmd.Command.
func (c *destroyControllerCommand) Run(ctxt *cmd.Context) error {
	client, err := c.newClient()
	if err != nil {
		return fmt.Errorf("could not create JIMM client: %v", err)
	}
	defer client.Close()

	resp, err := client.StartDestroyControllerJob(&params.DestroyControllerRequest{
		ControllerName: c.controllerName,
	})
	if err != nil {
		return err
	}

	if c.detach {
		fmt.Printf(`
destroy-controller job started.
You can track the progress via job-status with the job ID:
	juju [jaas] job-status %s

	`,
			resp.JobID,
		)
	} else {
		fmt.Printf(`
Starting destroy-controller job.

Should you cancel this process, you can track the progress via job-status with the job ID:
	juju [jaas] job-status %s

	`,
			resp.JobID,
		)
	}

	if c.detach {
		return nil
	}

	// Don't use c.out for the logs since c.out
	// attempts to format the output.

	poller := logPoller{
		client:              client,
		jobId:               resp.JobID,
		sleepBetweenGetLogs: sleepBetweenGetLogs,
		out:                 ctxt.Stdout,
		follow:              true,
	}

	return poller.watchJobLogs()
}

func (c *destroyControllerCommand) newClient() (JIMMAPI, error) {
	currentController, err := c.store.CurrentController()
	if err != nil {
		return nil, fmt.Errorf("could not determine controller: %v", err)
	}

	apiCaller, err := c.NewAPIRootWithDialOpts(c.store, currentController, "", nil)
	if err != nil {
		return nil, err
	}

	return jimmAPI.NewClient(apiCaller), nil
}
