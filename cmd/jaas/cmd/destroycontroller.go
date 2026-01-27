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
Requests the JIMM server to destroy a Juju controller.
The controller will be destroyed asynchronously.

By default the command will wait for the destroy-controller job to complete
while printing the job logs. Note that the logs will not follow the
--output flag and will always be printed to stdout. This can allow
you to send the initial output with the job ID to a file, while the
logs are streamed to stdout.

Use the --detach flag to start the bootstrap job and return immediately,
printing only the job ID, without waiting for the job to complete.

The argument denotes the name of the Juju controller to be destroyed.

Note that JIMM will internally do the following:
- download the juju CLI matching the controller version
- destroy the controller
- unregister the controller from JIMM

Destroying controllers on k8s clouds will only work on uju 3.6.10 or newer.
As a workaround, you can first unregister the controller and then destroy
it separately.
`
	destroyControllerExamples = `
	juju [jaas] destroy-controller <controller name>
	juju [jaas] destroy-controller mycontroller
	juju [jaas] destroy-controller mycontorller --no-prompt
`
)

// destroyControllerCommand starts a destroy-controller job on the controller.
type destroyControllerCommand struct {
	modelcmd.ControllerCommandBase
	out cmd.Output

	store                    jujuclient.ClientStore
	destroyControllerAPIFunc func() (JIMMAPI, error)

	controllerName string

	// Flags
	detach   bool
	noPrompt bool
}

// NewDestroyControllerStartCommand returns a command to start a job
// that will destroy a Juju controller.
func NewDestroyControllerStartCommand() cmd.Command {
	cmd := &destroyControllerCommand{
		store: jujuclient.NewFileClientStore(),
	}
	cmd.destroyControllerAPIFunc = cmd.newClient

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
	c.out.AddFlags(f, "yaml", map[string]cmd.Formatter{
		"yaml": cmd.FormatYaml,
		"json": cmd.FormatJson,
	})
	f.BoolVar(&c.detach, "detach", false, "If set, the command will start the destroy-controller job and return immediately with the job ID, without waiting for the job to complete.")
	f.BoolVar(&c.noPrompt, "no-prompt", false, "If set, the command will not prompt the user for the controller name before proceeding")
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
	ctxt.Warningf("This command will destroy the %q controller and all its resources", c.controllerName)
	if !c.noPrompt {
		err := jujucmd.UserConfirmName(c.controllerName, "controller", ctxt)
		if err != nil {
			return fmt.Errorf("controller destruction: %w", err)
		}
	}

	client, err := c.destroyControllerAPIFunc()
	if err != nil {
		return fmt.Errorf("could not create JIMM client: %v", err)
	}
	defer client.Close()

	resp, err := client.StartDestroyControllerJob(&params.DestroyControllerRequest{
		ControllerName: c.controllerName,
	})
	if err != nil {
		return fmt.Errorf("failed to start destroy controller job: %w", err)
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
