// Copyright 2015 Canonical Ltd.

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

type changePermCommand struct {
	commandBase

	path entityPathValue

	controller bool
	template   bool
	addRead    userSet
	removeRead userSet
	setRead    userSet
}

func newChangePermCommand() cmd.Command {
	return modelcmd.WrapBase(&changePermCommand{})
}

var changePermDoc = `
The change-perm command changes permissions of an model
(default) or controller (with the --controller flag) within JEM.

For example:

    juju jem change-perm --add-read=alice,bob johndoe/mymodel
`

func (c *changePermCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "change-perm",
		Args:    "<user>/<envname|controllername>",
		Purpose: "set permissions of JEM entity",
		Doc:     changePermDoc,
	}
}

func (c *changePermCommand) SetFlags(f *gnuflag.FlagSet) {
	f.BoolVar(&c.controller, "controller", false, "change ACL of controller not model")
	f.BoolVar(&c.template, "template", false, "change ACL of template not model")
	f.Var(&c.addRead, "add-read", "list of names to add to read ACL")
	f.Var(&c.removeRead, "remove-read", "list of names to remove from read ACL")
	f.Var(&c.setRead, "set-read", "set read ACL to this list")
}

func (c *changePermCommand) Init(args []string) error {
	// Validate and store the entity reference.
	if len(args) != 1 {
		return errgo.Newf("got %d arguments, want 1", len(args))
	}
	if err := c.path.Set(args[0]); err != nil {
		return errgo.Mask(err)
	}
	if c.setRead != nil && (len(c.addRead) != 0 || len(c.removeRead) != 0) {
		return errgo.Newf("cannot specify --set-read with either --add-read or --remove-read")
	}
	if c.setRead == nil && len(c.addRead) == 0 && len(c.removeRead) == 0 {
		return errgo.New("no permissions specified")
	}
	if c.template && c.controller {
		return errgo.New("cannot specify both --controller and --template")
	}
	return nil
}

func (c *changePermCommand) Run(ctxt *cmd.Context) error {
	client, err := c.newClient(ctxt)
	if err != nil {
		return errgo.Mask(err)
	}

	if c.setRead != nil {
		return c.setPerm(client, params.ACL{
			Read: c.setRead.slice(),
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
	for u := range c.addRead {
		newReadPerms[u] = true
	}
	for u := range c.removeRead {
		delete(newReadPerms, u)
	}
	return c.setPerm(client, params.ACL{
		Read: newReadPerms.slice(),
	})
}

func (c *changePermCommand) setPerm(client *jemclient.Client, acl params.ACL) error {
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

func (c *changePermCommand) getPerm(client *jemclient.Client) (params.ACL, error) {
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
