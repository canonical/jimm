// Copyright 2024 Canonical.

package cmd

import (
	"github.com/juju/cmd/v3"
	"github.com/juju/gnuflag"
	jujuapi "github.com/juju/juju/api"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/jujuclient"

	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/pkg/api"
	apiparams "github.com/canonical/jimm/v3/pkg/api/params"
)

var setControllerDeprecatedDoc = `
	set-controller-deprecated sets the deprecated status of a controller.

	Example:
		jimmctl set-controller-deprecated <name> --false 
`

// NewSetControllerDeprecatedCommand returns a command used to grant
// users access to audit logs.
func NewSetControllerDeprecatedCommand() cmd.Command {
	cmd := &setControllerDeprecatedCommand{
		store: jujuclient.NewFileClientStore(),
	}

	return modelcmd.WrapBase(cmd)
}

// setControllerDeprecatedCommand displays full
// model status.
type setControllerDeprecatedCommand struct {
	modelcmd.ControllerCommandBase
	out cmd.Output

	store    jujuclient.ClientStore
	dialOpts *jujuapi.DialOpts

	controllerName string
}

func (c *setControllerDeprecatedCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "set-controller-deprecated",
		Purpose: "Sets controller deprecated status.",
		Doc:     setControllerDeprecatedDoc,
	})
}

// SetFlags implements Command.SetFlags.
func (c *setControllerDeprecatedCommand) SetFlags(f *gnuflag.FlagSet) {
	c.CommandBase.SetFlags(f)
	c.out.AddFlags(f, "yaml", map[string]cmd.Formatter{
		"yaml": cmd.FormatYaml,
		"json": cmd.FormatJson,
	})
}

// Init implements the cmd.Command interface.
func (c *setControllerDeprecatedCommand) Init(args []string) error {
	if len(args) == 0 {
		return errors.E("missing controller name")
	}
	c.controllerName, args = args[0], args[1:]
	if len(args) > 0 {
		return errors.E("unknown arguments")
	}
	return nil
}

// Run implements Command.Run.
func (c *setControllerDeprecatedCommand) Run(ctxt *cmd.Context) error {
	currentController, err := c.store.CurrentController()
	if err != nil {
		return errors.E(err, "could not determine controller")
	}

	apiCaller, err := c.NewAPIRootWithDialOpts(c.store, currentController, "", c.dialOpts)
	if err != nil {
		return err
	}

	client := api.NewClient(apiCaller)

	info, err := client.SetControllerDeprecated(&apiparams.SetControllerDeprecatedRequest{
		Name:       c.controllerName,
		Deprecated: true,
	})
	if err != nil {
		return errors.E(err)
	}

	err = c.out.Write(ctxt, info)
	if err != nil {
		return errors.E(err)
	}
	return nil
}
