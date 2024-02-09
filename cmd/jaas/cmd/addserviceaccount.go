// Copyright 2024 Canonical Ltd.

package cmd

import (
	"github.com/juju/cmd/v3"
	"github.com/juju/gnuflag"
	jujuapi "github.com/juju/juju/api"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/jujuclient"

	"github.com/canonical/jimm/api"
	apiparams "github.com/canonical/jimm/api/params"
	"github.com/canonical/jimm/internal/errors"
)

var (
	addServiceCommandDoc = `
add-service-account binds a service account to your user, giving you administrator access over the service account.
Can only be run once per service account.
`
	addServiceCommandExamples = `
    juju add-service-account <client-id> 
`
)

// NewAddControllerCommand returns a command to add a service account
func NewAddServiceAccountCommand() cmd.Command {
	cmd := &addServiceAccountCommand{
		store: jujuclient.NewFileClientStore(),
	}

	return modelcmd.WrapBase(cmd)
}

// addServiceAccountCommand binds a service account to a user.
type addServiceAccountCommand struct {
	modelcmd.ControllerCommandBase
	out cmd.Output

	store    jujuclient.ClientStore
	dialOpts *jujuapi.DialOpts
	clientID string
}

// Info implements Command.Info.
func (c *addServiceAccountCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "add-service-account",
		Purpose:  "Add permission to manage a service account",
		Args:     "<client-id>",
		Examples: addServiceCommandExamples,
		Doc:      addServiceCommandDoc,
	})
}

// SetFlags implements the cmd.Command interface.
func (c *addServiceAccountCommand) SetFlags(f *gnuflag.FlagSet) {
	c.CommandBase.SetFlags(f)
	c.out.AddFlags(f, "yaml", map[string]cmd.Formatter{
		"yaml": cmd.FormatYaml,
		"json": cmd.FormatJson,
	})
}

// Init implements the cmd.Command interface.
func (c *addServiceAccountCommand) Init(args []string) error {
	if len(args) < 1 {
		return errors.E("clientID not specified")
	}
	c.clientID = args[0]
	if len(args) > 1 {
		return errors.E("too many args")
	}
	return nil
}

// Run implements Command.Run.
func (c *addServiceAccountCommand) Run(ctxt *cmd.Context) error {
	currentController, err := c.store.CurrentController()
	if err != nil {
		return errors.E(err, "could not determine controller")
	}

	apiCaller, err := c.NewAPIRootWithDialOpts(c.store, currentController, "", c.dialOpts)
	if err != nil {
		return err
	}

	params := apiparams.AddServiceAccountRequest{ClientID: c.clientID}
	client := api.NewClient(apiCaller)
	err = client.AddServiceAccount(&params)
	if err != nil {
		return errors.E(err)
	}

	err = c.out.Write(ctxt, "service account added successfully")
	if err != nil {
		return errors.E(err)
	}
	return nil
}
