// Copyright 2025 Canonical.

package cmd

import (
	"fmt"

	"github.com/juju/cmd/v3"
	"github.com/juju/gnuflag"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/names/v5"

	apiparams "github.com/canonical/jimm/v3/pkg/api/params"
)

const (
	listMigrationTargetsDoc = `
Requests a list of controllers connected to JIMM that are valid migration
targets for the specified model.

This is useful to obtain a filtered list of controllers that meets the following
criteria:
- The controller is not the current controller for the model.
- The controller can deploy to the the same cloud/region as the current controller.
- The controller is running a compatible Juju version i.e. newer than or equal to
  the current controller.
`
	listMigrationTargetsExamples = `
	juju list-migration-targets bb933db6-b151-417f-9a62-e3e80b4ebc9b
`
)

type listMigrationTargetsCommand struct {
	jaasCommandBase
	out cmd.Output

	modelTag string
}

// NewListMigrationTargetsCommand returns a command to list
// valid migration targets for an internal model migration.
func NewListMigrationTargetsCommand() cmd.Command {
	cmd := &listMigrationTargetsCommand{}
	cmd.SetClientStore(jujuclient.NewFileClientStore())

	return modelcmd.WrapBase(cmd)
}

// Init implements modelcmd.Command.
func (c *listMigrationTargetsCommand) Init(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("expected model uuid argument")
	}
	if !names.IsValidModel(args[0]) {
		return fmt.Errorf("invalid model uuid %q", args[0])
	}
	c.modelTag = names.NewModelTag(args[0]).String()

	return nil
}

// SetFlags implements modelcmd.Command.
func (c *listMigrationTargetsCommand) SetFlags(f *gnuflag.FlagSet) {
	c.CommandBase.SetFlags(f)
	c.out.AddFlags(f, "yaml", map[string]cmd.Formatter{
		"yaml": cmd.FormatYaml,
		"json": cmd.FormatJson,
	})
}

// Info implements modelcmd.Command.
func (c *listMigrationTargetsCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "list-migration-targets",
		Args:     "<model uuid>",
		Purpose:  "List migration targets for internal model migration.",
		Doc:      listMigrationTargetsDoc,
		Examples: listMigrationTargetsExamples,
	})
}

// Run implements modelcmd.Command.
func (c *listMigrationTargetsCommand) Run(ctxt *cmd.Context) error {
	req := apiparams.ListMigrationTargetsRequest{
		ModelTag: c.modelTag,
	}

	client, err := c.getJIMMAPI()
	if err != nil {
		return fmt.Errorf("could not create JIMM client: %w", err)
	}
	defer client.Close()

	resp, err := client.ListMigrationTargets(&req)
	if err != nil {
		return err
	}

	err = c.out.Write(ctxt, resp)
	if err != nil {
		return err
	}

	return nil
}
