// Copyright 2015-2016 Canonical Ltd.

package admincmd

import (
	"context"

	"github.com/juju/cmd"
	"gopkg.in/errgo.v1"
)

type revokeCommand struct {
	*commandBase

	path    entityPathValue
	aclName string
	users   userSet
}

func newRevokeCommand(c *commandBase) cmd.Command {
	return &revokeCommand{
		commandBase: c,
	}
}

var revokeDoc = `
The revoke command removes permissions for a set of users or groups from
an administrative function.

    jaas-admin revoke audit-log alice,bob
`

func (c *revokeCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "revoke",
		Args:    "<name> username[,username]...",
		Purpose: "revoke permissions of the administrative function",
		Doc:     revokeDoc,
	}
}

func (c *revokeCommand) Init(args []string) error {
	// Validate and store the entity reference.
	if len(args) == 0 {
		return errgo.Newf("no administrative function specified")
	}
	if len(args) == 1 {
		return errgo.Newf("no users specified")
	}
	if len(args) > 2 {
		return errgo.Newf("too many arguments")
	}
	c.aclName = args[0]
	c.users = make(userSet)
	if err := c.users.Set(args[1]); err != nil {
		return errgo.Notef(err, "invalid value %q", args[1])
	}
	return nil
}

func (c *revokeCommand) Run(ctxt *cmd.Context) error {
	ctx, cancel := wrapContext(ctxt)
	defer cancel()
	return c.runAdmin(ctx, ctxt)
}

func (c *revokeCommand) runAdmin(ctx context.Context, ctxt *cmd.Context) error {
	client, err := c.newACLClient(ctxt)
	if err != nil {
		return errgo.Mask(err)
	}
	defer client.Close()
	return errgo.Mask(client.Remove(ctx, c.aclName, c.users.slice()))
}
