// Copyright 2025 Canonical.

package cmd

import (
	"github.com/juju/cmd/v3"
	"github.com/juju/gnuflag"
	jujuapi "github.com/juju/juju/api"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/names/v5"
	jujuversion "github.com/juju/version/v2"

	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/pkg/api"
	apiparams "github.com/canonical/jimm/v3/pkg/api/params"
)

const (
	upgradeToDoc = `
Upgrades a controller to a specified version.
`
	upgradeToExample = `
    juju upgrade-to 3.6.11 2cb433a6-04eb-4ec4-9567-90426d20a004
`
)

// NewUpgradeToCommand returns a command to upgrade a controller to a specified version.
func NewUpgradeToCommand() cmd.Command {
	cmd := &upgradeToCommand{
		store: jujuclient.NewFileClientStore(),
	}
	cmd.jimmAPIFunc = cmd.newClient

	return modelcmd.WrapBase(cmd)
}

// upgradeToCommand upgrades a controller to a specified version.
type upgradeToCommand struct {
	modelcmd.ControllerCommandBase
	out cmd.Output

	store       jujuclient.ClientStore
	dialOpts    *jujuapi.DialOpts
	version     string
	modelUUID   string
	jimmAPIFunc func() (JIMMAPI, error)
}

func (c *upgradeToCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "upgrade-to",
		Args:     "<version> <model-uuid>",
		Purpose:  "Upgrades a controller to a specified version",
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
		return errors.E("missing required arguments: version and model UUID")
	}
	if len(args) > 2 {
		return errors.E("too many arguments")
	}
	c.version = args[0]
	c.modelUUID = args[1]

	// Validate version format
	if _, err := jujuversion.Parse(c.version); err != nil {
		return errors.E("invalid version format: " + c.version)
	}

	// Validate model UUID format
	if !names.IsValidModel(c.modelUUID) {
		return errors.E("invalid model UUID: " + c.modelUUID)
	}

	return nil
}

// Run implements Command.Run.
func (c *upgradeToCommand) Run(ctxt *cmd.Context) error {
	client, err := c.jimmAPIFunc()
	if err != nil {
		return errors.E(err, "failed to create JIMM client")
	}
	defer client.Close()

	modelTag := names.NewModelTag(c.modelUUID)
	resp, err := client.UpgradeTo(&apiparams.UpgradeToRequest{
		TargetControllerVersion: c.version,
		ModelTag:                modelTag.String(),
	})
	if err != nil {
		return errors.E(err)
	}
	if !resp.Success {
		err = c.out.Write(ctxt, resp)
		if err != nil {
			return errors.E(err)
		}
	}
	return nil
}

// newClient creates a new JIMM API client.
func (c *upgradeToCommand) newClient() (JIMMAPI, error) {
	currentController, err := c.store.CurrentController()
	if err != nil {
		return nil, errors.E(err, "could not determine controller")
	}

	apiCaller, err := c.NewAPIRootWithDialOpts(c.store, currentController, "", c.dialOpts)
	if err != nil {
		return nil, err
	}

	return api.NewClient(apiCaller), nil
}
