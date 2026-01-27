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

	"github.com/canonical/jimm/v3/pkg/api"
	apiparams "github.com/canonical/jimm/v3/pkg/api/params"
)

const (
	migrateInternalModelCommandDoc = `
The migrate-internal command migrates a model(s) between two controllers
in your JAAS system. This performs a model migration, but is named
"migrate-internal" to avoid confusion with the "migrate" command which migrates
a model to JAAS.

You may specify a model name (of the form owner/name) or model UUID.

`
	migrateInternalModelCommandExample = `
    juju migrate-internal mycontroller 2cb433a6-04eb-4ec4-9567-90426d20a004 fd469983-27c2-423b-bebf-84f616fb036b ...
    juju migrate-internal mycontroller user@domain.com/model-a user@domain.com/model-b ...
    juju migrate-internal mycontroller user@domain.com/model-a fd469983-27c2-423b-bebf-84f616fb036b ...

`
)

// NewMigrateInternalModelCommand returns a command to migrate internal models.
func NewMigrateInternalModelCommand() cmd.Command {
	cmd := &migrateInternalModelCommand{
		store: jujuclient.NewFileClientStore(),
	}

	cmd.jimmAPIFunc = cmd.newClient

	return modelcmd.WrapBase(cmd)
}

// migrateInternalModelCommand migrates a model between controllers within JAAS.
type migrateInternalModelCommand struct {
	modelcmd.ControllerCommandBase
	out cmd.Output

	store            jujuclient.ClientStore
	targetController string
	modelTargets     []string

	jimmAPIFunc func() (JIMMAPI, error)
}

// Info implements Command.Info.
func (c *migrateInternalModelCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "migrate-internal",
		Args:     "<controller name> <model uuid> [<model uuid>...]",
		Purpose:  "migrate models to another controller within JAAS",
		Doc:      migrateInternalModelCommandDoc,
		Examples: migrateInternalModelCommandExample,
	})
}

// SetFlags implements Command.SetFlags.
func (c *migrateInternalModelCommand) SetFlags(f *gnuflag.FlagSet) {
	c.CommandBase.SetFlags(f)
	c.out.AddFlags(f, "yaml", map[string]cmd.Formatter{
		"yaml": cmd.FormatYaml,
		"json": cmd.FormatJson,
	})
}

// Init implements the cmd.Command interface.
func (c *migrateInternalModelCommand) Init(args []string) error {
	if len(args) < 2 {
		return errors.New("missing controller name and model target arguments")
	}
	for i, arg := range args {
		if i == 0 {
			c.targetController = arg
			continue
		}
		c.modelTargets = append(c.modelTargets, arg)
	}
	return nil
}

// Run implements Command.Run.
func (c *migrateInternalModelCommand) Run(ctxt *cmd.Context) error {
	if c.jimmAPIFunc == nil {
		c.jimmAPIFunc = c.newClient
	}

	jimmAPI, err := c.jimmAPIFunc()
	if err != nil {
		return fmt.Errorf("could not create JIMM API client: %w", err)
	}
	defer jimmAPI.Close()

	specs := []apiparams.MigrateModelInfo{}
	for _, model := range c.modelTargets {
		specs = append(specs, apiparams.MigrateModelInfo{TargetModelNameOrUUID: model, TargetController: c.targetController})
	}
	req := apiparams.MigrateModelRequest{Specs: specs}
	events, err := jimmAPI.MigrateModel(&req)
	if err != nil {
		return fmt.Errorf("could not migrate models: %w", err)
	}

	err = c.out.Write(ctxt, events)
	if err != nil {
		return fmt.Errorf("could not write output: %w", err)
	}
	return nil
}

func (c *migrateInternalModelCommand) newClient() (JIMMAPI, error) {
	currentController, err := c.store.CurrentController()
	if err != nil {
		return nil, fmt.Errorf("could not determine controller: %w", err)
	}

	apiCaller, err := c.NewAPIRootWithDialOpts(c.store, currentController, "", nil)
	if err != nil {
		return nil, err
	}

	return api.NewClient(apiCaller), nil
}
