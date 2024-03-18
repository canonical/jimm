// Copyright 2021 Canonical Ltd.

package cmd

import (
	"github.com/juju/cmd/v3"
	"github.com/juju/gnuflag"
	jujuapi "github.com/juju/juju/api"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/names/v5"

	"github.com/canonical/jimm/api"
	apiparams "github.com/canonical/jimm/api/params"
	"github.com/canonical/jimm/internal/errors"
)

var modelStatusCommandDoc = `
	model-status command displays full model status.

	Example:
		jimmctl model-status <model uuid> 
		jimmctl model-status <model uuid> --format yaml
`

// NewModelStatusCommand returns a command to display full model status.
func NewModelStatusCommand() cmd.Command {
	cmd := &modelStatusCommand{
		store: jujuclient.NewFileClientStore(),
	}

	return modelcmd.WrapBase(cmd)
}

// modelStatusCommand displays full
// model status.
type modelStatusCommand struct {
	modelcmd.ControllerCommandBase
	out cmd.Output

	store     jujuclient.ClientStore
	dialOpts  *jujuapi.DialOpts
	modelUUID string
}

func (c *modelStatusCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "model-status",
		Purpose: "Displays full model status",
		Doc:     modelStatusCommandDoc,
	})
}

// SetFlags implements Command.SetFlags.
func (c *modelStatusCommand) SetFlags(f *gnuflag.FlagSet) {
	c.CommandBase.SetFlags(f)
	c.out.AddFlags(f, "yaml", map[string]cmd.Formatter{
		"yaml": cmd.FormatYaml,
		"json": cmd.FormatJson,
	})
}

// Init implements the cmd.Command interface.
func (c *modelStatusCommand) Init(args []string) error {
	if len(args) < 1 {
		return errors.E("missing model uuid")
	}
	c.modelUUID, args = args[0], args[1:]
	if len(args) > 0 {
		return errors.E("unknown arguments")
	}
	return nil
}

// Run implements Command.Run.
func (c *modelStatusCommand) Run(ctxt *cmd.Context) error {
	currentController, err := c.store.CurrentController()
	if err != nil {
		return errors.E(err, "could not determine controller")
	}

	modelTag := names.NewModelTag(c.modelUUID)
	apiCaller, err := c.NewAPIRootWithDialOpts(c.store, currentController, "", c.dialOpts)
	if err != nil {
		return err
	}

	client := api.NewClient(apiCaller)
	status, err := client.FullModelStatus(&apiparams.FullModelStatusRequest{
		ModelTag: modelTag.String(),
	})
	if err != nil {
		return errors.E(err)
	}

	err = c.out.Write(ctxt, status)
	if err != nil {
		return errors.E(err)
	}
	return nil
}
