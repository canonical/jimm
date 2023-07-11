// Copyright 2021 Canonical Ltd.

package cmd

import (
	"io/ioutil"

	"github.com/juju/cmd/v3"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/jujuclient"
	"sigs.k8s.io/yaml"

	apiparams "github.com/CanonicalLtd/jimm/api/params"
)

var (
	controllerInfoCommandDoc = `
	controller-info command writes controller information contained
	in the juju client store to a yaml file.

	If a --public-address flag is specified the output controller
	information will include this public address. The controller
	information will also omit the ca-certrificate.

	Example:
		jimmctl controller-info <name> <filename> 
		jimmctl controller-info --public-address=<dns-name>:443 <name> <filename> 
`
)

// NewControllerInfoCommand returns a command that writes
// controller information to a yaml file.
func NewControllerInfoCommand() cmd.Command {
	cmd := &controllerInfoCommand{
		store: jujuclient.NewFileClientStore(),
	}

	return modelcmd.WrapBase(cmd)
}

// controllerInfoCommand writes controller information
// to a yaml file.
type controllerInfoCommand struct {
	modelcmd.ControllerCommandBase

	store          jujuclient.ClientStore
	controllerName string
	publicAddress  string
	file           cmd.FileVar
}

func (c *controllerInfoCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "controller-info",
		Purpose: "Stores controller info to a yaml file",
		Doc:     controllerInfoCommandDoc,
	})
}

// SetFlags implements Command.SetFlags.
func (c *controllerInfoCommand) SetFlags(f *gnuflag.FlagSet) {
	c.CommandBase.SetFlags(f)
}

// Init implements the cmd.Command interface.
func (c *controllerInfoCommand) Init(args []string) error {
	if len(args) < 3 {
		return errors.New("controller name, filename or public address not specified")
	}
	c.controllerName, c.file.Path, c.publicAddress = args[0], args[1], args[2]
	if len(args) > 3 {
		return errors.New("too many args")
	}
	return nil
}

// Run implements Command.Run.
func (c *controllerInfoCommand) Run(ctxt *cmd.Context) error {
	controller, err := c.store.ControllerByName(c.controllerName)
	if err != nil {
		return errors.Mask(err)
	}
	account, err := c.store.AccountDetails(c.controllerName)
	if err != nil {
		return errors.Mask(err)
	}
	info := apiparams.AddControllerRequest{
		Name:         c.controllerName,
		APIAddresses: controller.APIEndpoints,
		Username:     account.User,
		Password:     account.Password,
	}
	if c.publicAddress != "" {
		info.PublicAddress = c.publicAddress
	} else {
		return errors.New("public address must be set")
	}
	data, err := yaml.Marshal(info)
	if err != nil {
		return errors.Mask(err)
	}
	err = ioutil.WriteFile(c.file.Path, data, 0666)
	if err != nil {
		return errors.Mask(err)
	}
	return nil
}
