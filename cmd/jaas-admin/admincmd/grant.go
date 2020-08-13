package admincmd

import (
	"context"
	"sort"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/gnuflag"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/names/v4"
	"gopkg.in/errgo.v1"

	"github.com/CanonicalLtd/jimm/params"
)

type grantCommand struct {
	commandBase

	path    entityPathValue
	aclName string

	admin      bool
	controller bool
	set        bool
	users      userSet
}

func newGrantCommand() cmd.Command {
	return modelcmd.WrapBase(&grantCommand{})
}

var grantDoc = `
The grant command grants permissions for a set of users or groups to
read a model (default), controller or an administrative function within
the managing server.  Note that if someone can read a model from the
managing server they can access that model and make changes to it.

For example, this will allow alice and bob to read the model johndoe/mymodel.

    jaas admin grant johndoe/mymodel alice,bob

If the --set flag is provided, the ACLs will be overwritten rather than added.

    jaas admin grant johndoe/mymodel --set fred,bob

If the --admin flag is provided, the ACL that is changed will be for
accessing an administrative function.

    jaas admin grant --admin audit-log alice,bob
`

func (c *grantCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "grant",
		Args:    "<name> username[,username]",
		Purpose: "grant permissions of managing server entity",
		Doc:     grantDoc,
	}
}

func (c *grantCommand) SetFlags(f *gnuflag.FlagSet) {
	c.commandBase.SetFlags(f)
	f.BoolVar(&c.controller, "controller", false, "change ACL of controller not model")
	f.BoolVar(&c.admin, "admin", false, "change an admin ACL")
	f.BoolVar(&c.set, "set", false, "overwrite the current acl")
}

func (c *grantCommand) Init(args []string) error {
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

func (c *grantCommand) Run(ctxt *cmd.Context) error {
	ctx, cancel := wrapContext(ctxt)
	defer cancel()

	if c.admin {
		return c.runAdmin(ctx, ctxt)
	}
	return c.run(ctx, ctxt)
}

func (c *grantCommand) run(ctx context.Context, ctxt *cmd.Context) error {
	client, err := c.newClient(ctxt)
	if err != nil {
		return errgo.Mask(err)
	}
	defer client.Close()

	if c.set {
		return c.setPerm(ctx, client, params.ACL{
			Read: c.users.slice(),
		})
	}
	currentACL, err := c.getPerm(ctx, client)
	if err != nil {
		return errgo.Mask(err)
	}
	newReadPerms := make(userSet)
	for _, u := range currentACL.Read {
		newReadPerms[u] = true
	}
	for u := range c.users {
		newReadPerms[u] = true
	}
	return c.setPerm(ctx, client, params.ACL{
		Read: newReadPerms.slice(),
	})
}

func (c *grantCommand) setPerm(ctx context.Context, client *client, acl params.ACL) error {
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

func (c *grantCommand) getPerm(ctx context.Context, client *client) (params.ACL, error) {
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

func (c *grantCommand) runAdmin(ctx context.Context, ctxt *cmd.Context) error {
	client, err := c.newACLClient(ctxt)
	if err != nil {
		return errgo.Mask(err)
	}
	defer client.Close()

	if c.set {
		return errgo.Mask(client.Set(ctx, c.aclName, c.users.slice()))
	}
	return errgo.Mask(client.Add(ctx, c.aclName, c.users.slice()))
}

// userSet represents a set of users and implements gnuflag.Value
// so can be used as a command line flag argument.
type userSet map[string]bool

func (us userSet) String() string {
	return strings.Join(us.slice(), ",")
}

func (us userSet) slice() []string {
	slice := make([]string, 0, len(us))
	for s := range us {
		slice = append(slice, s)
	}
	sort.Strings(slice)
	return slice
}

// Set implements gnuflag.Value.Set.
func (us *userSet) Set(s string) error {
	m := make(userSet)
	if s == "" {
		*us = m
		return nil
	}
	slice := strings.Split(s, ",")
	for _, u := range slice {
		u = strings.TrimSpace(u)
		if u == "" {
			return errgo.Newf("empty user found")
		}
		if !names.IsValidUser(u) {
			return errgo.Newf("invalid user name %q", u)
		}
		m[u] = true
	}
	*us = m
	return nil
}
