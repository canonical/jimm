// Copyright 2015-2016 Canonical Ltd.

package jemcmd

import (
	"sort"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/names"
	"gopkg.in/errgo.v1"
	"launchpad.net/gnuflag"

	"github.com/CanonicalLtd/jem/jemclient"
	"github.com/CanonicalLtd/jem/params"
)

type grantCommand struct {
	commandBase

	path entityPathValue

	controller bool
	template   bool
	set        bool
	users      userSet
}

func newGrantCommand() cmd.Command {
	return modelcmd.WrapBase(&grantCommand{})
}

var grantDoc = `
The grant command grants permissions for a set of users or groups
to read a model (default), controller, or template within the managing server.
Note that if someone can read a model from the managing server they can
access that model and make changes to it.

For example, this will allow alice and bob to read the model johndoe/mymodel.

    jaas model grant johndoe/mymodel alice,bob

If the --set flag is provided, the ACLs will be overwritten rather than added.

    jaas model grant johndoe/mymodel --set fred,bob
`

func (c *grantCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "grant",
		Args:    "<user>/<modelname|controllername> username[,username]",
		Purpose: "grant permissions of managing server entity",
		Doc:     grantDoc,
	}
}

func (c *grantCommand) SetFlags(f *gnuflag.FlagSet) {
	f.BoolVar(&c.controller, "controller", false, "change ACL of controller not model")
	f.BoolVar(&c.template, "template", false, "change ACL of template not model")
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
	if err := c.path.Set(args[0]); err != nil {
		return errgo.Mask(err)
	}
	if c.template && c.controller {
		return errgo.New("cannot specify both --controller and --template")
	}
	c.users = make(userSet)
	if err := c.users.Set(args[1]); err != nil {
		return errgo.Notef(err, "invalid value %q", args[1])
	}
	return nil
}

func (c *grantCommand) Run(ctxt *cmd.Context) error {
	client, err := c.newClient(ctxt)
	if err != nil {
		return errgo.Mask(err)
	}

	if c.set {
		return c.setPerm(client, params.ACL{
			Read: c.users.slice(),
		})
	}
	currentACL, err := c.getPerm(client)
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
	return c.setPerm(client, params.ACL{
		Read: newReadPerms.slice(),
	})
}

func (c *grantCommand) setPerm(client *jemclient.Client, acl params.ACL) error {
	var err error
	switch {
	case c.controller:
		err = client.SetControllerPerm(&params.SetControllerPerm{
			EntityPath: c.path.EntityPath,
			ACL:        acl,
		})
	case c.template:
		err = client.SetTemplatePerm(&params.SetTemplatePerm{
			EntityPath: c.path.EntityPath,
			ACL:        acl,
		})
	default:
		err = client.SetModelPerm(&params.SetModelPerm{
			EntityPath: c.path.EntityPath,
			ACL:        acl,
		})
	}
	return errgo.Mask(err)
}

func (c *grantCommand) getPerm(client *jemclient.Client) (params.ACL, error) {
	var acl params.ACL
	var err error
	switch {
	case c.controller:
		acl, err = client.GetControllerPerm(&params.GetControllerPerm{
			EntityPath: c.path.EntityPath,
		})
	case c.template:
		acl, err = client.GetTemplatePerm(&params.GetTemplatePerm{
			EntityPath: c.path.EntityPath,
		})
	default:
		acl, err = client.GetModelPerm(&params.GetModelPerm{
			EntityPath: c.path.EntityPath,
		})
	}
	return acl, errgo.Mask(err)
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
		if !names.IsValidUserName(u) {
			return errgo.Newf("invalid user name %q", u)
		}
		m[u] = true
	}
	*us = m
	return nil
}
