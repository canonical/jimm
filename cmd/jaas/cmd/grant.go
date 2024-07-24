// Copyright 2024 Canonical Ltd.

package cmd

import (
	"fmt"

	"github.com/juju/cmd/v3"
	jujuapi "github.com/juju/juju/api"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/jujuclient"

	"github.com/canonical/jimm/internal/errors"
	api "github.com/canonical/jimmapi"
	apiparams "github.com/canonical/jimmapi/params"
)

var (
	grantCommandDoc = `
grant-service-account-access grants administrator access over a service account to the given groups/identities.
`
	grantCommandExamples = `
    juju grant-service-account-access 00000000-0000-0000-0000-000000000000 user-foo group-bar
`
)

// NewGrantCommand returns a command to grant admin access to a service account to given groups/identities.
func NewGrantCommand() cmd.Command {
	cmd := &grantCommand{
		store: jujuclient.NewFileClientStore(),
	}

	return modelcmd.WrapBase(cmd)
}

// grantCommand grants admin access to a service account to given groups/identities.
type grantCommand struct {
	modelcmd.ControllerCommandBase
	out cmd.Output

	store    jujuclient.ClientStore
	dialOpts *jujuapi.DialOpts

	clientID string
	entities []string
}

// Info implements Command.Info.
func (c *grantCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "grant-service-account-access",
		Args:     "<client-id> (<user>|<group>) [(<user>|<group>) ...]",
		Purpose:  "Grants administrator access over a service account",
		Examples: grantCommandExamples,
		Doc:      grantCommandDoc,
	})
}

// Init implements the cmd.Command interface.
func (c *grantCommand) Init(args []string) error {
	if len(args) < 1 {
		return errors.E("client ID not specified")
	}
	c.clientID = args[0]
	if len(args) < 2 {
		return errors.E("user/group not specified")
	}
	c.entities = args[1:]
	return nil
}

// Run implements Command.Run.
func (c *grantCommand) Run(ctxt *cmd.Context) error {
	currentController, err := c.store.CurrentController()
	if err != nil {
		return errors.E(err, "could not determine controller")
	}

	apiCaller, err := c.NewAPIRootWithDialOpts(c.store, currentController, "", c.dialOpts)
	if err != nil {
		return errors.E(err, "failed to dial the controller")
	}

	params := apiparams.GrantServiceAccountAccess{
		ClientID: c.clientID,
		Entities: c.entities,
	}

	client := api.NewClient(apiCaller)
	err = client.GrantServiceAccountAccess(&params)
	if err != nil {
		return errors.E(err)
	}
	fmt.Fprintln(ctxt.Stdout, "access granted")
	return nil
}
