// Copyright 2021 Canonical Ltd.

package cmd

import (
	"github.com/juju/cmd"
	"github.com/juju/gnuflag"
	jujuapi "github.com/juju/juju/api"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/jujuclient"

	"github.com/CanonicalLtd/jimm/api"
	"github.com/CanonicalLtd/jimm/internal/errors"
)

var listControllersComandDoc = `
	list-controllers command displays controller information
	for all controllers known to JIMM.

	Example:
		jimmctl controllers 
		jimmctl controllers --format json --output ~/tmp/controllers.json
`

// NewListControllersCommand returns a command to list controller information.
func NewListControllersCommand() cmd.Command {
	cmd := &listControllersCommand{
		store: jujuclient.NewFileClientStore(),
	}

	return modelcmd.WrapBase(cmd)
}

// listControllersCommand shows controller information
// for all controllers known to JIMM.
type listControllersCommand struct {
	modelcmd.ControllerCommandBase
	out cmd.Output

	store    jujuclient.ClientStore
	dialOpts *jujuapi.DialOpts
}

func (c *listControllersCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "controllers",
		Purpose: "Lists all controllers known to JIMM.",
		Doc:     listControllersComandDoc,
		Aliases: []string{"list-controllers"},
	})
}

// SetFlags implements Command.SetFlags.
func (c *listControllersCommand) SetFlags(f *gnuflag.FlagSet) {
	c.CommandBase.SetFlags(f)
	c.out.AddFlags(f, "yaml", map[string]cmd.Formatter{
		"yaml": cmd.FormatYaml,
		"json": cmd.FormatJson,
	})
}

// Run implements Command.Run.
func (c *listControllersCommand) Run(ctxt *cmd.Context) error {
	currentController, err := c.store.CurrentController()
	if err != nil {
		return errors.E(err, "could not determine controller")
	}
	apiCaller, err := c.NewAPIRootWithDialOpts(c.store, currentController, "", c.dialOpts)
	if err != nil {
		return err
	}

	client := api.NewClient(apiCaller)
	controllers, err := client.ListControllers()
	if err != nil {
		return errors.E(err)
	}

	err = c.out.Write(ctxt, controllers)
	if err != nil {
		return errors.E(err)
	}
	return nil
}
