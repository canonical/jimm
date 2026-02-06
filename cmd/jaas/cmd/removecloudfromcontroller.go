// Copyright 2026 Canonical.

package cmd

import (
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
	removeCloudFromControllerCommandDoc = `
Removes the specified cloud from the specified controller in JIMM.
`

	removeCloudFromControllerCommandExample = `
    juju remove-cloud mycontroller mycloud
`
)

// NewRemoveCloudFromControllerCommand returns a command to
// remove a cloud from a specific controller in JIMM.
func NewRemoveCloudFromControllerCommand() cmd.Command {
	cmd := &removeCloudFromControllerCommand{}
	cmd.SetClientStore(jujuclient.NewFileClientStore())

	return modelcmd.WrapBase(cmd)
}

// addControllerCommand adds a controller.
type removeCloudFromControllerCommand struct {
	modelcmd.ControllerCommandBase
	out cmd.Output

	// cloudName is the name of the cloud to remove.
	cloudName string

	// targetControllerName is the name of the controller in JIMM where the cloud
	// should be removed from.
	targetControllerName string

	jimmAPIFunc func() (JIMMAPI, error)
}

// Info implements Command.Info.
func (c *removeCloudFromControllerCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "remove-cloud",
		Args:     "<controller_name> <cloud_name>",
		Purpose:  "Remove cloud from specific controller in jimm",
		Doc:      removeCloudFromControllerCommandDoc,
		Examples: removeCloudFromControllerCommandExample,
	})
}

// SetFlags implements Command.SetFlags.
func (c *removeCloudFromControllerCommand) SetFlags(f *gnuflag.FlagSet) {
	c.CommandBase.SetFlags(f)
	c.out.AddFlags(f, "yaml", map[string]cmd.Formatter{
		"yaml": cmd.FormatYaml,
		"json": cmd.FormatJson,
	})
}

// Init implements the cmd.Command interface.
func (c *removeCloudFromControllerCommand) Init(args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("missing arguments")
	}
	if len(args) > 2 {
		return fmt.Errorf("too many arguments")
	}
	c.targetControllerName = args[0]
	if ok := names.IsValidControllerName(c.targetControllerName); !ok {
		return fmt.Errorf("invalid controller name %q", c.targetControllerName)
	}
	c.cloudName = args[1]
	if ok := names.IsValidCloud(c.cloudName); !ok {
		return fmt.Errorf("invalid cloud name %q", c.cloudName)
	}

	return nil
}

// Run implements Command.Run.
func (c *removeCloudFromControllerCommand) Run(ctxt *cmd.Context) error {
	err := c.removeCloudFromController(ctxt)
	if err != nil {
		return fmt.Errorf("error removing cloud from controller: %w", err)
	}

	return nil
}

func (c *removeCloudFromControllerCommand) removeCloudFromController(ctxt *cmd.Context) error {
	if c.jimmAPIFunc == nil {
		c.jimmAPIFunc = c.newClient
	}

	client, err := c.jimmAPIFunc()
	if err != nil {
		return err
	}
	defer client.Close()

	params := &apiparams.RemoveCloudFromControllerRequest{
		CloudTag:       "cloud-" + c.cloudName,
		ControllerName: c.targetControllerName,
	}

	err = client.RemoveCloudFromController(params)
	if err != nil {
		return err
	}

	ctxt.Infof("Cloud %q removed from controller %q.", c.cloudName, c.targetControllerName)
	return nil
}

func (c *removeCloudFromControllerCommand) newClient() (JIMMAPI, error) {
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
