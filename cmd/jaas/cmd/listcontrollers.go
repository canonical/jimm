// Copyright 2025 Canonical.

package cmd

import (
	"fmt"

	"github.com/juju/cmd/v3"
	"github.com/juju/gnuflag"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/jujuclient"

	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/pkg/api"
)

const (
	listControllersCommandDoc = `
Displays controller information for all controllers known to JIMM.
`
	listControllersCommandExample = `
    juju controllers 
    juju controllers --format json
`
)

// NewListControllersCommand returns a command to list controller information.
func NewListControllersCommand() cmd.Command {
	cmd := &listControllersCommand{
		store: jujuclient.NewFileClientStore(),
	}
	cmd.jimmAPIFunc = cmd.newClient

	return modelcmd.WrapBase(cmd)
}

// listControllersCommand shows controller information
// for all controllers known to JIMM.
type listControllersCommand struct {
	modelcmd.ControllerCommandBase
	out cmd.Output

	store jujuclient.ClientStore

	jimmAPIFunc func() (JIMMAPI, error)
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
	if c.jimmAPIFunc == nil {
		c.jimmAPIFunc = c.newClient
	}

	client, err := c.jimmAPIFunc()
	if err != nil {
		return err
	}
	defer client.Close()

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

func (c *listControllersCommand) newClient() (JIMMAPI, error) {
	currentController, err := c.store.CurrentController()
	if err != nil {
		return nil, errors.E(fmt.Errorf("could not determine controller: %v", err))
	}

	apiCaller, err := c.NewAPIRootWithDialOpts(c.store, currentController, "", nil)
	if err != nil {
		return nil, err
	}

	return api.NewClient(apiCaller), nil
}
