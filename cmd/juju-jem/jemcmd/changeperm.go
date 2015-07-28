// Copyright 2015 Canonical Ltd.

package jemcmd

import (
	"sort"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/names"
	"gopkg.in/errgo.v1"
	"launchpad.net/gnuflag"

	"github.com/CanonicalLtd/jem/params"
)

type changePermCommand struct {
	commandBase

	path entityPathValue

	stateServer bool
	addRead     userSet
	removeRead  userSet
	setRead     userSet
}

var changePermDoc = `
The change-perm command changes permissions of an environment
(default) or state server (with the --server flag) within JEM.

For example:

    juju jem change-perm --add-read=alice,bob johndoe/myenv
`

func (c *changePermCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "change-perm",
		Args:    "<user>/<envname|servername>",
		Purpose: "set permissions of JEM entity",
		Doc:     changePermDoc,
	}
}

func (c *changePermCommand) SetFlags(f *gnuflag.FlagSet) {
	f.BoolVar(&c.stateServer, "server", false, "change perm of state server not environment")
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
	return nil
}

func (c *changePermCommand) Run(ctxt *cmd.Context) error {
	client, err := c.newClient()
	if err != nil {
		return errgo.Mask(err)
	}
	defer client.Close()

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

func (c *changePermCommand) setPerm(client *jemClient, acl params.ACL) error {
	logger.Infof("setPerm %#v\n", acl)
	var err error
	if c.stateServer {
		err = client.SetStateServerPerm(&params.SetStateServerPerm{
			EntityPath: c.path.EntityPath,
			ACL:        acl,
		})
	} else {
		err = client.SetEnvironmentPerm(&params.SetEnvironmentPerm{
			EntityPath: c.path.EntityPath,
			ACL:        acl,
		})
	}
	return errgo.Mask(err)
}

func (c *changePermCommand) getPerm(client *jemClient) (params.ACL, error) {
	var acl params.ACL
	var err error
	if c.stateServer {
		acl, err = client.GetStateServerPerm(&params.GetStateServerPerm{
			EntityPath: c.path.EntityPath,
		})
	} else {
		acl, err = client.GetEnvironmentPerm(&params.GetEnvironmentPerm{
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
