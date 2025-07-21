// Copyright 2025 Canonical.

package cmd

import (
	"time"

	"github.com/juju/cmd/v3"
	"github.com/juju/gnuflag"
	jujuapi "github.com/juju/juju/api"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/jujuclient"

	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/pkg/api"
	"github.com/canonical/jimm/v3/pkg/api/params"
)

const (
	bootstrapStatusCommandDoc = `
Displays logs for a bootstrap job.
`
	bootstrapStatusCommandExample = `
    juju bootstrap-status 2cb433a6-04eb-4ec4-9567-90426d20a004 
`
)

// sleepBetweenGetLogs is the duration to wait between successive calls to get logs for a bootstrap job.
const sleepBetweenGetLogs = 1 * time.Second

// NewbootstrapStatusCommand returns a command to display logs for a bootstrap job.
func NewBootstrapStatusCommand() cmd.Command {
	cmd := &bootstrapStatusCommand{
		store: jujuclient.NewFileClientStore(),
	}

	return modelcmd.WrapBase(cmd)
}

// bootstrapStatusCommand displays logs for a bootstrap job.
type bootstrapStatusCommand struct {
	modelcmd.ControllerCommandBase

	store    jujuclient.ClientStore
	dialOpts *jujuapi.DialOpts
	jobId    string
	client   JIMMClient

	sleepBetweenGetLogs time.Duration
	follow              bool
}

// Info implements cmd.Info interface.
func (c *bootstrapStatusCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "bootstrap-status",
		Args:     "<job uuid>",
		Purpose:  "Displays logs for a bootstrap job",
		Doc:      bootstrapStatusCommandDoc,
		Examples: bootstrapStatusCommandExample,
	})
}

// SetFlags implements cmd.SetFlags interface.
func (c *bootstrapStatusCommand) SetFlags(f *gnuflag.FlagSet) {
	c.CommandBase.SetFlags(f)
	f.BoolVar(&c.follow, "f", false, "follow the logs of the bootstrap job")
}

// Init implements the cmd.Command interface.
func (c *bootstrapStatusCommand) Init(args []string) error {
	if len(args) < 1 {
		return errors.E("missing job id")
	}
	c.jobId, args = args[0], args[1:]
	if len(args) > 0 {
		return errors.E("unknown arguments")
	}
	currentController, err := c.store.CurrentController()
	if err != nil {
		return errors.E(err, "could not determine controller")
	}

	apiCaller, err := c.NewAPIRootWithDialOpts(c.store, currentController, "", c.dialOpts)
	if err != nil {
		return err
	}

	c.client = api.NewClient(apiCaller)

	c.sleepBetweenGetLogs = sleepBetweenGetLogs
	return nil
}

// Run implements cmd.Command.Run interface.
func (c *bootstrapStatusCommand) Run(ctxt *cmd.Context) error {
	watermark := 0
	for {
		response, err := c.client.BootstrapStatus(&params.BootstrapStatusRequest{
			JobID:     c.jobId,
			Watermark: watermark,
		})
		if err != nil {
			return errors.E(err, "failed to get bootstrap status")
		}
		switch response.Status {
		case params.StatusRunning:
			for _, log := range response.Logs {
				_, err = ctxt.Stdout.Write([]byte(log + "\n"))
				if err != nil {
					return errors.E(err, "failed to write bootstrap log")
				}
			}
			watermark = response.Watermark
		case params.StatusSuccessful:
			_, err = ctxt.Stdout.Write([]byte("Bootstrap job completed successfully.\n"))
			if err != nil {
				return errors.E(err, "failed to write bootstrap success message")
			}
			return nil
		case params.StatusFailed:
			_, err = ctxt.Stdout.Write([]byte("Bootstrap job failed: " + response.Error + "\n"))
			if err != nil {
				return errors.E(err, "failed to write bootstrap error")
			}
			return nil
		case params.StatusPending:
			_, err := ctxt.Stdout.Write([]byte("Bootstrap job is pending...\n"))
			if err != nil {
				return errors.E(err, "failed to write bootstrap pending message")
			}
		default:
			return errors.E("unknown bootstrap job status: %s", response.Status)
		}
		if !c.follow {
			return nil
		}
		time.Sleep(c.sleepBetweenGetLogs)
	}

}
