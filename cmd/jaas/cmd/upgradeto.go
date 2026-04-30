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
	upgradeToDoc = `
Upgrades a model by migrating it to a specific controller
and upgrades the model to the controller's version.
`
	upgradeToExample = `
    juju upgrade-to myController 2cb433a6-04eb-4ec4-9567-90426d20a004
`
)

// NewUpgradeToCommand returns a command to upgrade a controller to a specified version.
func NewUpgradeToCommand() cmd.Command {
	cmd := &upgradeToCommand{}
	cmd.SetClientStore(jujuclient.NewFileClientStore())

	return modelcmd.WrapBase(cmd)
}

// upgradeToCommand upgrades a controller to a specified version.
type upgradeToCommand struct {
	jaasCommandBase
	out cmd.Output

	controllerName string
	modelUUID      string
}

func (c *upgradeToCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "upgrade-to",
		Args:     "<controller-name> <model-uuid>",
		Purpose:  "Upgrades a model",
		Doc:      upgradeToDoc,
		Examples: upgradeToExample,
	})
}

// SetFlags implements Command.SetFlags.
func (c *upgradeToCommand) SetFlags(f *gnuflag.FlagSet) {
	c.CommandBase.SetFlags(f)
	c.out.AddFlags(f, "yaml", map[string]cmd.Formatter{
		"yaml": cmd.FormatYaml,
		"json": cmd.FormatJson,
	})
}

// Init implements the cmd.Command interface.
func (c *upgradeToCommand) Init(args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("missing required arguments: controller name and model UUID")
	}
	if len(args) > 2 {
		return fmt.Errorf("too many arguments")
	}
	c.controllerName = args[0]
	c.modelUUID = args[1]

	// Validate model UUID format
	if !names.IsValidModel(c.modelUUID) {
		return fmt.Errorf("invalid model UUID: %s", c.modelUUID)
	}

	return nil
}

// Run implements Command.Run.
func (c *upgradeToCommand) Run(ctxt *cmd.Context) error {
	client, err := c.getJIMMAPI()
	if err != nil {
		return fmt.Errorf("failed to create JIMM client: %w", err)
	}
	defer client.Close()

	modelTag := names.NewModelTag(c.modelUUID)
	resp, err := client.UpgradeTo(&apiparams.UpgradeToRequest{
		TargetControllerName: c.controllerName,
		ModelTag:             modelTag.String(),
	})
	if err != nil {
		return err
	}

	err = c.out.Write(ctxt, resp)
	if err != nil {
		return err
	}

	return nil
}
