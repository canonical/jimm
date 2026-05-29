// Copyright 2025 Canonical.

package cmd

import (
	"github.com/juju/cmd/v3"
	"github.com/juju/gnuflag"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/jujuclient"
)

const (
	listControllersCommandDoc = `
Displays controller information for all controllers known to JIMM.

For JAAS admins, this will also display controllers that are in the process of being bootstrapped.
`
	listControllersCommandExample = `
    juju controllers
    juju controllers --format json
`
)

// NewListControllersCommand returns a command to list controller information.
func NewListControllersCommand() cmd.Command {
	cmd := &listControllersCommand{}
	cmd.SetClientStore(jujuclient.NewFileClientStore())

	return modelcmd.WrapBase(cmd)
}

// listControllersCommand shows controller information
// for all controllers known to JIMM.
type listControllersCommand struct {
	jaasCommandBase
	out cmd.Output
}

func (c *listControllersCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "controllers",
		Purpose:  "Lists all controllers known to JIMM.",
		Doc:      listControllersCommandDoc,
		Examples: listControllersCommandExample,
		Aliases:  []string{"list-controllers"},
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
	client, err := c.getJIMMAPI()
	if err != nil {
		return err
	}
	defer client.Close()

	controllers, err := client.ListControllers()
	if err != nil {
		return err
	}

	err = c.out.Write(ctxt, controllers)
	if err != nil {
		return err
	}
	return nil
}
