// Copyright 2025 Canonical.

package cmd

import (
	"fmt"

	"github.com/juju/cmd/v3"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/names/v5"

	"github.com/canonical/jimm/v3/pkg/api"
	apiparams "github.com/canonical/jimm/v3/pkg/api/params"
)

const (
	updateMigratedModelCommandDoc = `
Updates a model known to JIMM that has been migrated
externally to a different JAAS controller.
`
	updateMigratedModelCommandExample = `
    juju update-migrated-model mycontroller e0bf3abf-7029-4e48-9c26-68a7b6e02947
`
)

// NewUpdateMigratedModelCommand returns a command to update the controller
// running a model.
func NewUpdateMigratedModelCommand() cmd.Command {
	cmd := &updateMigratedModelCommand{}
	cmd.SetClientStore(jujuclient.NewFileClientStore())

	return modelcmd.WrapBase(cmd)
}

// updateMigratedModelCommand updates the controller running a model.
type updateMigratedModelCommand struct {
	modelcmd.ControllerCommandBase

	req apiparams.UpdateMigratedModelRequest

	jimmAPIFunc func() (JIMMAPI, error)
}

// Info implements the cmd.Command interface.
func (c *updateMigratedModelCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "update-migrated-model",
		Args:     "<controller name> <model uuid>",
		Purpose:  "Update the controller running a model.",
		Doc:      updateMigratedModelCommandDoc,
		Examples: updateMigratedModelCommandExample,
	})
}

// Init implements the cmd.Command interface.
func (c *updateMigratedModelCommand) Init(args []string) error {
	switch len(args) {
	default:
		return fmt.Errorf("too many args")
	case 0:
		return fmt.Errorf("controller not specified")
	case 1:
		return fmt.Errorf("model uuid not specified")
	case 2:
	}

	c.req.TargetController = args[0]
	if !names.IsValidModel(args[1]) {
		return fmt.Errorf("invalid model uuid")
	}
	c.req.ModelTag = names.NewModelTag(args[1]).String()
	return nil
}

// Run implements Command.Run.
func (c *updateMigratedModelCommand) Run(ctxt *cmd.Context) error {
	if c.jimmAPIFunc == nil {
		c.jimmAPIFunc = c.newClient
	}

	client, err := c.jimmAPIFunc()
	if err != nil {
		return err
	}
	defer client.Close()

	if err := client.UpdateMigratedModel(&c.req); err != nil {
		return err
	}
	return nil
}

func (c *updateMigratedModelCommand) newClient() (JIMMAPI, error) {
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
