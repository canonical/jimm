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
	grantCommandDoc = `
grant command grants administrator access over a service account to the given groups/identities.

Example:
	juju service-account grant <client-id> (<user>|<group>) [(<user>|<group>) ...]
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

func (c *grantCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "grant",
		Purpose: "Grants administrator access over a service account to the given groups/identities",
		Doc:     grantCommandDoc,
	})
}

// SetFlags implements Command.SetFlags.
func (c *grantCommand) SetFlags(f *gnuflag.FlagSet) {
	c.CommandBase.SetFlags(f)
	c.out.AddFlags(f, "yaml", map[string]cmd.Formatter{
		"yaml": cmd.FormatYaml,
		"json": cmd.FormatJson,
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
		return err
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

	err = c.out.Write(ctxt, "access granted")
	if err != nil {
		return errors.E(err)
	}
	return nil
}
