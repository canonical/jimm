package admincmd

import (
	"context"
	"sort"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/gnuflag"
	"github.com/juju/names/v4"
	"gopkg.in/errgo.v1"
)

type grantCommand struct {
	*commandBase

	path    entityPathValue
	aclName string

	admin      bool
	controller bool
	set        bool
	users      userSet
}

func newGrantCommand(c *commandBase) cmd.Command {
	return &grantCommand{
		commandBase: c,
	}
}

var grantDoc = `
The grant command grants permissions for a set of users or groups to an
administrative function within the managing server.  

    jaas admin grant audit-log alice,bob

If the --set flag is provided, the ACLs will be overwritten rather than added.

    jaas-admin grant audit-log --set alice,bob
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
	f.BoolVar(&c.set, "set", false, "overwrite the current acl")
}

func (c *grantCommand) Init(args []string) error {
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

func (c *grantCommand) Run(ctxt *cmd.Context) error {
	ctx, cancel := wrapContext(ctxt)
	defer cancel()
	return c.runAdmin(ctx, ctxt)
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
