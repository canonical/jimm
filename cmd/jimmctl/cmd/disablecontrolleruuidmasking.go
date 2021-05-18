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

var disableControllerUUIDMaskingCommandDoc = `
	disable-controller-uuid-masking command disables the masking of the 
	real controller UUID with JIMM's UUID.

	Example:
		jimmctl disable-controller-uuid-masking 
`

// NewDisableControllerUUIDMaskingCommand returns a command to
// disable controller uuid masking.
func NewDisableControllerUUIDMaskingCommand() cmd.Command {
	cmd := &disableControllerUUIDMaskingCommand{
		store: jujuclient.NewFileClientStore(),
	}

	return modelcmd.WrapBase(cmd)
}

// disableControllerUUIDMaskingCommand disables controller uuid masking.
type disableControllerUUIDMaskingCommand struct {
	modelcmd.ControllerCommandBase
	out cmd.Output

	store    jujuclient.ClientStore
	dialOpts *jujuapi.DialOpts
}

func (c *disableControllerUUIDMaskingCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "disable-controller-uuid-masking",
		Purpose: "Disables controller UUID masking.",
		Doc:     disableControllerUUIDMaskingCommandDoc,
	})
}

// SetFlags implements Command.SetFlags.
func (c *disableControllerUUIDMaskingCommand) SetFlags(f *gnuflag.FlagSet) {
	c.CommandBase.SetFlags(f)
	c.out.AddFlags(f, "yaml", map[string]cmd.Formatter{
		"yaml": cmd.FormatYaml,
		"json": cmd.FormatJson,
	})
}

// Run implements Command.Run.
func (c *disableControllerUUIDMaskingCommand) Run(ctxt *cmd.Context) error {
	currentController, err := c.store.CurrentController()
	if err != nil {
		return errors.E(err, "could not determine controller")
	}
	apiCaller, err := c.NewAPIRootWithDialOpts(c.store, currentController, "", c.dialOpts)
	if err != nil {
		return err
	}

	client := api.NewClient(apiCaller)
	err = client.DisableControllerUUIDMasking()
	if err != nil {
		return errors.E(err)
	}
	return nil
}
