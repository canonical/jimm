// Copyright 2025 Canonical.

package cmd

import (
	"errors"
	"fmt"

	"github.com/juju/cmd/v3"
	"github.com/juju/gnuflag"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/names/v5"

	"github.com/canonical/jimm/v3/pkg/api"
	apiparams "github.com/canonical/jimm/v3/pkg/api/params"
)

const (
	importModelCommandDoc = `
Imports a model running on a controller into JIMM's state.

When importing, it is necessary for JIMM to contain a set of cloud credentials
that represent a user's access to the incoming model's cloud.

The --owner command is necessary when importing a model created by a
local user and it will switch the model owner to the desired external user.
`
	importModelCommandExample = `
    juju import-model mycontroller ac30d6ae-0bed-4398-bba7-75d49e39f189
    juju import-model mycontroller ac30d6ae-0bed-4398-bba7-75d49e39f189 --owner user@canonical.com
`
)

// NewImportModelCommand returns a command to import a model.
func NewImportModelCommand() cmd.Command {
	cmd := &importModelCommand{}
	cmd.SetClientStore(jujuclient.NewFileClientStore())
	cmd.jimmAPIFunc = cmd.newClient

	return modelcmd.WrapBase(cmd)
}

// importModelCommand imports a model.
type importModelCommand struct {
	modelcmd.ControllerCommandBase

	req apiparams.ImportModelRequest

	jimmAPIFunc func() (JIMMAPI, error)
}

// Info implements the cmd.Command interface.
func (c *importModelCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "import-model",
		Args:     "<controller name> <model uuid>",
		Purpose:  "Import a model to jimm.",
		Doc:      importModelCommandDoc,
		Examples: importModelCommandExample,
		Aliases:  []string{"register-model"},
	})
}

// SetFlags implements Command.SetFlags.
func (c *importModelCommand) SetFlags(f *gnuflag.FlagSet) {
	c.CommandBase.SetFlags(f)
	f.StringVar(&c.req.Owner, "owner", "", "switch the model owner to the desired user")
}

// Init implements the cmd.Command interface.
func (c *importModelCommand) Init(args []string) error {
	switch len(args) {
	default:
		return fmt.Errorf("too many args")
	case 0:
		return fmt.Errorf("controller not specified")
	case 1:
		return fmt.Errorf("model uuid not specified")
	case 2:
	}

	c.req.Controller = args[0]
	if !names.IsValidModel(args[1]) {
		return errors.New("invalid model uuid")
	}
	c.req.ModelTag = names.NewModelTag(args[1]).String()
	return nil
}

// Run implements Command.Run.
func (c *importModelCommand) Run(ctxt *cmd.Context) error {
	if c.jimmAPIFunc == nil {
		c.jimmAPIFunc = c.newClient
	}

	jimmAPI, err := c.jimmAPIFunc()
	if err != nil {
		return fmt.Errorf("could not create JIMM API client: %w", err)
	}
	defer jimmAPI.Close()

	if err := jimmAPI.ImportModel(&c.req); err != nil {
		return fmt.Errorf("could not import model: %w", err)
	}
	return nil
}

func (c *importModelCommand) newClient() (JIMMAPI, error) {
	currentController, err := c.ClientStore().CurrentController()
	if err != nil {
		return nil, fmt.Errorf("could not determine controller: %w", err)
	}

	apiCaller, err := c.NewAPIRootWithDialOpts(c.ClientStore(), currentController, "", nil)
	if err != nil {
		return nil, err
	}

	return api.NewClient(apiCaller), nil
}
