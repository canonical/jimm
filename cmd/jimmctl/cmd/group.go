// Copyright 2021 Canonical Ltd.

package cmd

import (
	"github.com/juju/cmd/v3"
	jujucmdv3 "github.com/juju/cmd/v3"
	jujuapi "github.com/juju/juju/api"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/jujuclient"

	"github.com/CanonicalLtd/jimm/api"
	apiparams "github.com/CanonicalLtd/jimm/api/params"
	"github.com/CanonicalLtd/jimm/internal/errors"
)

var (
	groupDoc = `
	group command enables group management for jimm
`

	addGroupDoc = `
	add-group command adds group to jimm.

	Example:
		jimmctl auth group add <name> 
`
)

// NewGroupCommand returns a command for group management.
func NewGroupCommand() *jujucmdv3.SuperCommand {
	cmd := jujucmd.NewSuperCommand(jujucmdv3.SuperCommandParams{
		Name:    "group",
		Doc:     groupDoc,
		Purpose: "Group management.",
	})
	cmd.Register(newAddGroupCommand())

	return cmd
}

// newAddGroupCommand returns a command to add a group.
func newAddGroupCommand() cmd.Command {
	cmd := &addGroupCommand{
		store: jujuclient.NewFileClientStore(),
	}

	return modelcmd.WrapBase(cmd)
}

// addGroupCommand adds a group.
type addGroupCommand struct {
	modelcmd.ControllerCommandBase
	out cmd.Output

	store    jujuclient.ClientStore
	dialOpts *jujuapi.DialOpts

	name string
}

func (c *addGroupCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "add",
		Purpose: "Add group to jimm",
		Doc:     addGroupDoc,
	})
}

// Init implements the cmd.Command interface.
func (c *addGroupCommand) Init(args []string) error {
	if len(args) < 1 {
		return errors.E("group name not specified")
	}
	c.name, args = args[0], args[1:]
	if len(args) > 0 {
		return errors.E("too many args")
	}
	return nil
}

// Run implements Command.Run.
func (c *addGroupCommand) Run(ctxt *cmd.Context) error {
	currentController, err := c.store.CurrentController()
	if err != nil {
		return errors.E(err, "could not determine controller")
	}

	apiCaller, err := c.NewAPIRootWithDialOpts(c.store, currentController, "", c.dialOpts)
	if err != nil {
		return err
	}

	params := apiparams.AddGroupRequest{
		Name: c.name,
	}

	client := api.NewClient(apiCaller)
	err = client.AddGroup(&params)
	if err != nil {
		return errors.E(err)
	}

	return nil
}
