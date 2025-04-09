// Copyright 2025 Canonical.

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

const (
	unregisterControllerCommandDoc = `
Deregisters a controller from JIMM.
`

	unregisterControllerCommandExample = `
    juju unregister-controller mycontroller 
    juju unregister-controller mycontroller --force
`
)

// NewUnregisterControllerCommand returns a command to unregister a controller.
func NewUnregisterControllerCommand() cmd.Command {
	cmd := &unregisterControllerCommand{
		store: jujuclient.NewFileClientStore(),
	}

	return modelcmd.WrapBase(cmd)
}

// unregisterControllerCommand unregister a controller.
type unregisterControllerCommand struct {
	modelcmd.ControllerCommandBase
	out cmd.Output

	store    jujuclient.ClientStore
	dialOpts *jujuapi.DialOpts
	params   apiparams.RemoveControllerRequest
}

func (c *unregisterControllerCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "unregister-controller",
		Args:     "<name>",
		Purpose:  "Remove controller from jimm",
		Doc:      unregisterControllerCommandDoc,
		Examples: unregisterControllerCommandExample,
	})
}

// SetFlags implements Command.SetFlags.
func (c *unregisterControllerCommand) SetFlags(f *gnuflag.FlagSet) {
	c.CommandBase.SetFlags(f)
	c.out.AddFlags(f, "yaml", map[string]cmd.Formatter{
		"yaml": cmd.FormatYaml,
		"json": cmd.FormatJson,
	})
	f.BoolVar(&c.params.Force, "force", false, "force unregister a controller")
}

// Init implements the cmd.Command interface.
func (c *unregisterControllerCommand) Init(args []string) error {
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
func (c *unregisterControllerCommand) Run(ctxt *cmd.Context) error {
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
