// Copyright 2026 Canonical.

package cmd

import (
	"fmt"

	"github.com/juju/cmd/v3"
	"github.com/juju/gnuflag"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/jujuclient"

	apiparams "github.com/canonical/jimm/v3/pkg/api/params"
)

const (
	setControllerDeprecatedDoc = `
Sets the deprecated status of a controller.
`
	setControllerDeprecatedExample = `
    juju set-controller-deprecated mycontroller
`
)

// NewSetControllerDeprecatedCommand returns a command used to grant
// users access to audit logs.
func NewSetControllerDeprecatedCommand() cmd.Command {
	cmd := &setControllerDeprecatedCommand{}
	cmd.SetClientStore(jujuclient.NewFileClientStore())

	return modelcmd.WrapBase(cmd)
}

// setControllerDeprecatedCommand displays full
// model status.
type setControllerDeprecatedCommand struct {
	jaasCommandBase
	out cmd.Output

	controllerName string
}

func (c *setControllerDeprecatedCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "set-controller-deprecated",
		Args:     "<controller name>",
		Purpose:  "Sets controller deprecated status.",
		Doc:      setControllerDeprecatedDoc,
		Examples: setControllerDeprecatedExample,
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
		return fmt.Errorf("missing controller name")
	}
	c.controllerName, args = args[0], args[1:]
	if len(args) > 0 {
		return fmt.Errorf("unknown arguments")
	}
	return nil
}

// Run implements Command.Run.
func (c *setControllerDeprecatedCommand) Run(ctxt *cmd.Context) error {
	client, err := c.getJIMMAPI()
	if err != nil {
		return err
	}
	defer client.Close()

	info, err := client.SetControllerDeprecated(&apiparams.SetControllerDeprecatedRequest{
		Name:       c.controllerName,
		Deprecated: true,
	})
	if err != nil {
		return err
	}

	err = c.out.Write(ctxt, info)
	if err != nil {
		return err
	}
	return nil
}
