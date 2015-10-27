// Copyright 2015 Canonical Ltd.

package jemcmd

import (
	"github.com/juju/cmd"
	"github.com/juju/juju/cmd/envcmd"
	"gopkg.in/errgo.v1"
	"launchpad.net/gnuflag"

	"github.com/CanonicalLtd/jem/params"
)

type removeCommand struct {
	commandBase

	paths       []entityPathValue
	stateServer bool
	template    bool
}

func newRemoveCommand() cmd.Command {
	return envcmd.WrapBase(&removeCommand{})
}

var removeDoc = `
The remove command removes environments, servers or templates.
`

func (c *removeCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "remove",
		Args:    "[<user>/<name>...]",
		Purpose: "remove entities",
		Doc:     removeDoc,
	}
}

func (c *removeCommand) Init(args []string) error {
	for _, p := range args {
		var path entityPathValue
		if err := path.Set(p); err != nil {
			return errgo.Mask(err)
		}
		c.paths = append(c.paths, path)
	}
	return nil
}

func (c *removeCommand) SetFlags(f *gnuflag.FlagSet) {
	f.BoolVar(&c.stateServer, "server", false, "remove state servers not environments")
	f.BoolVar(&c.template, "template", false, "remove templates not environments")
}

func (c *removeCommand) Run(ctxt *cmd.Context) error {
	client, err := c.newClient(ctxt)
	if err != nil {
		return errgo.Mask(err)
	}
	typeString := "environments"
	f := func(path entityPathValue) error {
		return client.DeleteEnvironment(&params.DeleteEnvironment{
			EntityPath: path.EntityPath,
		})
	}
	if c.stateServer {
		typeString = "servers"
		f = func(path entityPathValue) error {
			return client.DeleteJES(&params.DeleteJES{
				EntityPath: path.EntityPath,
			})
		}
	}
	if c.template {
		typeString = "templates"
		f = func(path entityPathValue) error {
			return client.DeleteTemplate(&params.DeleteTemplate{
				EntityPath: path.EntityPath,
			})
		}
	}
	var failed bool
	for _, path := range c.paths {
		ctxt.Verbosef("removing %s", path)
		if err := f(path); err != nil {
			failed = true
			ctxt.Infof("cannot remove %s: %s", path, err)
		}
	}
	if failed {
		return errgo.Newf("not all %s removed successfully", typeString)
	}
	return nil
}
