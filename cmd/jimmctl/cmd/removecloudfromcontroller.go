// Copyright 2021 Canonical Ltd.

package cmd

import (
	"fmt"

	"github.com/juju/cmd/v3"
	"github.com/juju/gnuflag"
	jujuapi "github.com/juju/juju/api"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/names/v5"

	"github.com/canonical/jimm/v3/pkg/api"
	apiparams "github.com/canonical/jimm/v3/pkg/api/params"

	"github.com/canonical/jimm/v3/internal/errors"
)

var (
	removeCloudFromControllerCommandDoc = `
	remove-cloud-from-controller command removes the specified cloud from the 
	specified controller in jimm.

	Example:
		jimmctl remove-cloud-from-controller <controller_name> <cloud_name> 
`
)

// NewAddControllerCommand returns a command to add a cloud to a specific
// controller in JIMM.
func NewRemoveCloudFromControllerCommand() cmd.Command {
	cmd := &removeCloudFromControllerCommand{
		store: jujuclient.NewFileClientStore(),
	}
	cmd.removeCloudFromControllerAPIFunc = cmd.cloudAPI

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

	removeCloudFromControllerAPIFunc func() (removeCloudFromControllerAPI, error)
	store                            jujuclient.ClientStore
	dialOpts                         *jujuapi.DialOpts
}

type removeCloudFromControllerAPI interface {
	RemoveCloudFromController(params *apiparams.RemoveCloudFromControllerRequest) error
}

// Info implements Command.Info.
func (c *removeCloudFromControllerCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "remove-cloud-from-controller",
		Purpose: "Remove cloud from specific controller in jimm",
		Doc:     removeCloudFromControllerCommandDoc,
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
		return errors.E("missing arguments")
	}
	if len(args) > 2 {
		return errors.E("too many arguments")
	}
	c.targetControllerName = args[0]
	if ok := names.IsValidControllerName(c.targetControllerName); !ok {
		return errors.E("invalid controller name %q", c.targetControllerName)
	}
	c.cloudName = args[1]
	if ok := names.IsValidCloud(c.cloudName); !ok {
		return errors.E("invalid cloud name %q", c.cloudName)
	}

	return nil
}

// Run implements Command.Run.
func (c *removeCloudFromControllerCommand) Run(ctxt *cmd.Context) error {
	err := c.removeCloudFromController(ctxt)
	if err != nil {
		return errors.E(err, fmt.Sprintf("error removing cloud from controller: %v", err))
	}

	return nil
}

func (c *removeCloudFromControllerCommand) removeCloudFromController(ctxt *cmd.Context) error {
	client, err := c.removeCloudFromControllerAPIFunc()
	if err != nil {
		return errors.E(err)
	}

	params := &apiparams.RemoveCloudFromControllerRequest{
		CloudTag:       "cloud-" + c.cloudName,
		ControllerName: c.targetControllerName,
	}

	err = client.RemoveCloudFromController(params)
	if err != nil {
		return errors.E(err)
	}

	ctxt.Infof("Cloud %q removed from controller %q.", c.cloudName, c.targetControllerName)
	return nil
}

func (c *removeCloudFromControllerCommand) cloudAPI() (removeCloudFromControllerAPI, error) {
	currentController, err := c.store.CurrentController()
	if err != nil {
		return nil, errors.E(err, "could not determine the current controller")
	}
	apiCaller, err := c.NewAPIRootWithDialOpts(c.store, currentController, "", c.dialOpts)
	if err != nil {
		return nil, err
	}

	return api.NewClient(apiCaller), nil
}
