// Copyright 2021 Canonical Ltd.

package cmd

import (
	"github.com/juju/cmd/v3"
	"github.com/juju/gnuflag"
	jujuapi "github.com/juju/juju/api"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/jujuclient"

	"github.com/CanonicalLtd/jimm/api"
	apiparams "github.com/CanonicalLtd/jimm/api/params"
	"github.com/CanonicalLtd/jimm/internal/errors"
)

var (
	removeControllerCommandDoc = `
	remove-controller command removes a controller from jimm.

	Example:
		jimmctl remove-controller <name> 
		jimmctl remove-controller <name> --force
`
)

// NewRemoveControllerCommand returns a command to remove a controller.
func NewRemoveControllerCommand() cmd.Command {
	cmd := &removeControllerCommand{
		store: jujuclient.NewFileClientStore(),
	}

	return modelcmd.WrapBase(cmd)
}

// removeControllerCommand remove a controller.
type removeControllerCommand struct {
	modelcmd.ControllerCommandBase
	out cmd.Output

	store    jujuclient.ClientStore
	dialOpts *jujuapi.DialOpts
	params   apiparams.RemoveControllerRequest
}

func (c *removeControllerCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "remove-controller",
		Purpose: "Remove controller from jimm",
		Doc:     removeControllerCommandDoc,
	})
}

// SetFlags implements Command.SetFlags.
func (c *removeControllerCommand) SetFlags(f *gnuflag.FlagSet) {
	c.CommandBase.SetFlags(f)
	c.out.AddFlags(f, "yaml", map[string]cmd.Formatter{
		"yaml": cmd.FormatYaml,
		"json": cmd.FormatJson,
	})
	f.BoolVar(&c.params.Force, "force", false, "force remove a controller")
}

// Init implements the cmd.Command interface.
func (c *removeControllerCommand) Init(args []string) error {
	if len(args) < 1 {
		return errors.E("controller name not specified")
	}
	c.params.Name = args[0]
	if len(args) > 1 {
		return errors.E("too many args")
	}
	return nil
}

// Run implements Command.Run.
func (c *removeControllerCommand) Run(ctxt *cmd.Context) error {
	currentController, err := c.store.CurrentController()
	if err != nil {
		return errors.E(err, "could not determine controller")
	}

	apiCaller, err := c.NewAPIRootWithDialOpts(c.store, currentController, "", c.dialOpts)
	if err != nil {
		return err
	}
	client := api.NewClient(apiCaller)
	info, err := client.RemoveController(&c.params)
	if err != nil {
		return errors.E(err)
	}

	err = c.out.Write(ctxt, info)
	if err != nil {
		return errors.E(err)
	}
	return nil
}
