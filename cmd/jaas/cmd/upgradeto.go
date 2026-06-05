// Copyright 2025 Canonical.

package cmd

import (
	"fmt"
	"io"

	"github.com/juju/cmd/v3"
	"github.com/juju/gnuflag"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/cmd/output"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/names/v5"

	apiparams "github.com/canonical/jimm/v3/pkg/api/params"
)

const (
	upgradeToDoc = `
Upgrades models by migrating them to a specific controller
and upgrades the models to the controller's version.
`
	upgradeToExample = `
    juju upgrade-to myController 2cb433a6-04eb-4ec4-9567-90426d20a004
    juju upgrade-to myController 2cb433a6-04eb-4ec4-9567-90426d20a004 83cf3d62-ab16-4cb2-8e2f-df111fca1a32
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
	modelUUIDs     []string
}

func (c *upgradeToCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "upgrade-to",
		Args:     "<controller-name> <model-uuid> [<model-uuid>...]",
		Purpose:  "Upgrades a model",
		Doc:      upgradeToDoc,
		Examples: upgradeToExample,
	})
}

// SetFlags implements Command.SetFlags.
func (c *upgradeToCommand) SetFlags(f *gnuflag.FlagSet) {
	c.CommandBase.SetFlags(f)
	c.out.AddFlags(f, "tabular", map[string]cmd.Formatter{
		"yaml":    cmd.FormatYaml,
		"json":    cmd.FormatJson,
		"tabular": c.formatTabular,
	})
}

// Init implements the cmd.Command interface.
func (c *upgradeToCommand) Init(args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("missing required arguments: controller name and at least one model UUID")
	}
	c.controllerName = args[0]
	c.modelUUIDs = args[1:]

	for _, modelUUID := range c.modelUUIDs {
		if !names.IsValidModel(modelUUID) {
			return fmt.Errorf("invalid model UUID: %s", modelUUID)
		}
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

	req := &apiparams.UpgradeToRequest{
		TargetControllerName: c.controllerName,
		ModelUUIDs:           c.modelUUIDs,
	}

	resp, err := client.UpgradeTo(req)
	if err != nil {
		return fmt.Errorf("upgrade-to request failed: %w", err)
	}
	if len(resp.Results) != len(req.ModelUUIDs) {
		return fmt.Errorf("invalid upgrade-to response: got %d results for %d model UUIDs", len(resp.Results), len(req.ModelUUIDs))
	}

	if writeErr := c.out.Write(ctxt, resp); writeErr != nil {
		return writeErr
	}

	return nil
}

func (c *upgradeToCommand) formatTabular(writer io.Writer, value any) error {
	resp, ok := value.(apiparams.UpgradeToResponse)
	if !ok {
		return fmt.Errorf("expected apiparams.UpgradeToResponse, got %T", value)
	}

	tw := output.TabWriter(writer)
	w := output.Wrapper{TabWriter: tw}

	w.PrintHeaders(output.EmphasisHighlight.DefaultBold, "Model UUID", "Status", "Error")
	for i, modelUUID := range c.modelUUIDs {
		status := "success"
		errMsg := ""
		if resp.Results[i].Error != nil {
			status = "failed"
			errMsg = resp.Results[i].Error.Message
		}
		w.Println(modelUUID, status, errMsg)
	}
	w.Flush()

	return nil
}
