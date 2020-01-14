// Copyright 2015-2016 Canonical Ltd.

package admincmd

import (
	"context"
	"fmt"

	"github.com/juju/cmd"
	"github.com/juju/gnuflag"
	"github.com/juju/juju/cmd/modelcmd"
	"gopkg.in/errgo.v1"

	"github.com/CanonicalLtd/jimm/params"
)

type revokeCommand struct {
	commandBase

	path    entityPathValue
	aclName string

	controller bool
	admin      bool
	users      userSet
}

func newRevokeCommand() cmd.Command {
	return modelcmd.WrapBase(&revokeCommand{})
}

var revokeDoc = `
The revoke command removes permissions for a set of users
or groups to read a model (default) or controller within the managing server.
Note that if a user access is revoked, that user may still have access
if they are a member of a group that is still part of the read ACL.

For example, to remove alice and bob from the read ACL of the model johndoe/mymodel,
assuming they are currently mentioned in the ACL:

    jaas admin revoke johndoe/mymodel alice,bob

If the --admin flag is provided, the ACL that is changed will be for
accessing an administrative function.

    jaas admin grant --admin audit-log alice,bob
`

func (c *revokeCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "revoke",
		Args:    "<name> username[,username]...",
		Purpose: "revoke permissions of the managing server entity",
		Doc:     revokeDoc,
	}
}

func (c *revokeCommand) SetFlags(f *gnuflag.FlagSet) {
	f.BoolVar(&c.controller, "controller", false, "change ACL of controller not model")
	f.BoolVar(&c.admin, "admin", false, "change an admin ACL")
}

func (c *revokeCommand) Init(args []string) error {
	// Validate and store the entity reference.
	if len(args) == 0 {
		return errgo.Newf("no model or controller specified")
	}
	if len(args) == 1 {
		return errgo.Newf("no users specified")
	}
	if len(args) > 2 {
		return errgo.Newf("too many arguments")
	}
	if c.admin {
		c.aclName = args[0]
	} else {
		if err := c.path.Set(args[0]); err != nil {
			return errgo.Mask(err)
		}
	}
	c.users = make(userSet)
	if err := c.users.Set(args[1]); err != nil {
		return errgo.Notef(err, "invalid value %q", args[1])
	}
	return nil
}

func (c *revokeCommand) Run(ctxt *cmd.Context) error {
	ctx, cancel := wrapContext(ctxt)
	defer cancel()

	if c.admin {
		return c.runAdmin(ctx, ctxt)
	}
	return c.run(ctx, ctxt)
}

func (c *revokeCommand) run(ctx context.Context, ctxt *cmd.Context) error {
	client, err := c.newClient(ctxt)
	if err != nil {
		return errgo.Mask(err)
	}
	defer client.Close()
	currentACL, err := c.getPerm(ctx, client)
	if err != nil {
		return errgo.Mask(err)
	}
	newReadPerms := make(userSet)
	for _, u := range currentACL.Read {
		newReadPerms[u] = true
	}
	for u := range c.users {
		if _, ok := newReadPerms[u]; !ok {
			fmt.Fprintf(ctxt.Stdout, "User %q was not granted before revoke.\n", u)
		} else {
			delete(newReadPerms, u)
		}
	}
	return c.setPerm(ctx, client, params.ACL{
		Read: newReadPerms.slice(),
	})
}

func (c *revokeCommand) setPerm(ctx context.Context, client *client, acl params.ACL) error {
	var err error
	switch {
	case c.controller:
		err = client.SetControllerPerm(ctx, &params.SetControllerPerm{
			EntityPath: c.path.EntityPath,
			ACL:        acl,
		})
	default:
		err = client.SetModelPerm(ctx, &params.SetModelPerm{
			EntityPath: c.path.EntityPath,
			ACL:        acl,
		})
	}
	return errgo.Mask(err)
}

func (c *revokeCommand) getPerm(ctx context.Context, client *client) (params.ACL, error) {
	var acl params.ACL
	var err error
	switch {
	case c.controller:
		acl, err = client.GetControllerPerm(ctx, &params.GetControllerPerm{
			EntityPath: c.path.EntityPath,
		})
	default:
		acl, err = client.GetModelPerm(ctx, &params.GetModelPerm{
			EntityPath: c.path.EntityPath,
		})
	}
	return acl, errgo.Mask(err)
}

func (c *revokeCommand) runAdmin(ctx context.Context, ctxt *cmd.Context) error {
	client, err := c.newACLClient(ctxt)
	if err != nil {
		return errgo.Mask(err)
	}
	defer client.Close()
	return errgo.Mask(client.Remove(ctx, c.aclName, c.users.slice()))
}
