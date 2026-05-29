// Copyright 2026 Canonical.

package cmd

import (
	"fmt"

	"github.com/juju/cmd/v3"
	"github.com/juju/gnuflag"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/jujuclient"
)

const (
	showControllerCommandDoc = `
Displays information about a controller known to JIMM.

For controllers with an active bootstrap status, some fields will be empty/missing.
`
	showControllerCommandExample = `
    juju jaas show-controller my-controller
    juju jaas show-controller my-controller --format json
`
)

// NewShowControllerCommand returns a command to display controller information.
func NewShowControllerCommand() cmd.Command {
	cmd := &showControllerCommand{}
	cmd.SetClientStore(jujuclient.NewFileClientStore())

	return modelcmd.WrapBase(cmd)
}

// showControllerCommand displays information about a controller known to JIMM.
type showControllerCommand struct {
	jaasCommandBase
	out cmd.Output

	controllerName string
}

func (c *showControllerCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "show-controller",
		Args:     "<controller name>",
		Purpose:  "Displays information about a controller",
		Doc:      showControllerCommandDoc,
		Examples: showControllerCommandExample,
	})
}

// SetFlags implements Command.SetFlags.
func (c *showControllerCommand) SetFlags(f *gnuflag.FlagSet) {
	c.CommandBase.SetFlags(f)
	c.out.AddFlags(f, "yaml", map[string]cmd.Formatter{
		"yaml": cmd.FormatYaml,
		"json": cmd.FormatJson,
	})
}

// Init implements the cmd.Command interface.
func (c *showControllerCommand) Init(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("missing controller name")
	}
	c.controllerName, args = args[0], args[1:]
	if len(args) > 0 {
		return fmt.Errorf("unknown arguments: %v", args)
	}
	return nil
}

// Run implements Command.Run.
func (c *showControllerCommand) Run(ctxt *cmd.Context) error {
	client, err := c.getJIMMAPI()
	if err != nil {
		return err
	}
	defer client.Close()

	info, err := client.ShowController(c.controllerName)
	if err != nil {
		return err
	}

	return c.out.Write(ctxt, info)
}
