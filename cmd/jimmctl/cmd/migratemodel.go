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

const (
	migrateModelCommandDoc = `
The migrate commands migrates a model, or many models between two controllers
registered within JIMM. 

You may specify a model name (of the form owner/name) or model UUID.

`
	migrateModelCommandExample = `
    jimmctl migrate mycontroller 2cb433a6-04eb-4ec4-9567-90426d20a004 fd469983-27c2-423b-bebf-84f616fb036b ...
    jimmctl migrate mycontroller user@domain.com/model-a user@domain.com/model-b ...
    jimmctl migrate mycontroller user@domain.com/model-a fd469983-27c2-423b-bebf-84f616fb036b ...

`
)

// NewMigrateModelCommand returns a command to migrate models.
func NewMigrateModelCommand() cmd.Command {
	cmd := &migrateModelCommand{
		store: jujuclient.NewFileClientStore(),
	}

	return modelcmd.WrapBase(cmd)
}

// migrateModelCommand migrates a model.
type migrateModelCommand struct {
	modelcmd.ControllerCommandBase
	out cmd.Output

	store            jujuclient.ClientStore
	dialOpts         *jujuapi.DialOpts
	targetController string
	modelTargets     []string
}

func (c *migrateModelCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "migrate",
		Args:     "<controller name> <model uuid> [<model uuid>...]",
		Purpose:  "Migrate models to the target controller",
		Doc:      migrateModelCommandDoc,
		Examples: migrateModelCommandExample,
	})
}

// SetFlags implements Command.SetFlags.
func (c *migrateModelCommand) SetFlags(f *gnuflag.FlagSet) {
	c.CommandBase.SetFlags(f)
	c.out.AddFlags(f, "yaml", map[string]cmd.Formatter{
		"yaml": cmd.FormatYaml,
		"json": cmd.FormatJson,
	})
}

// Init implements the cmd.Command interface.
func (c *migrateModelCommand) Init(args []string) error {
	if len(args) < 2 {
		return errors.E("Missing controller name and model target arguments")
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
func (c *migrateModelCommand) Run(ctxt *cmd.Context) error {
	currentController, err := c.store.CurrentController()
	if err != nil {
		return errors.E(err, "could not determine controller")
	}

	apiCaller, err := c.NewAPIRootWithDialOpts(c.store, currentController, "", c.dialOpts)
	if err != nil {
		return err
	}

	client := api.NewClient(apiCaller)
	specs := []apiparams.MigrateModelInfo{}
	for _, model := range c.modelTargets {
		specs = append(specs, apiparams.MigrateModelInfo{TargetModelNameOrUUID: model, TargetController: c.targetController})
	}
	req := apiparams.MigrateModelRequest{Specs: specs}
	events, err := client.MigrateModel(&req)
	if err != nil {
		return err
	}

	err = c.out.Write(ctxt, events)
	if err != nil {
		return err
	}
	return nil
}
