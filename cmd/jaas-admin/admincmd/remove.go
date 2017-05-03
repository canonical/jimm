// Copyright 2015 Canonical Ltd.

package admincmd

import (
	"github.com/juju/cmd"
	"github.com/juju/gnuflag"
	"github.com/juju/juju/cmd/modelcmd"
	"gopkg.in/errgo.v1"

	"github.com/CanonicalLtd/jem/params"
)

type removeCommand struct {
	commandBase

	paths      []entityPathValue
	controller bool
	force      bool
}

func newRemoveCommand() cmd.Command {
	return modelcmd.WrapBase(&removeCommand{})
}

var removeDoc = `
The remove command removes models or controllers.
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
	f.BoolVar(&c.controller, "controller", false, "remove controllers not models")
	f.BoolVar(&c.force, "f", false, "force removal of live controller")
	f.BoolVar(&c.force, "force", false, "")
}

func (c *removeCommand) Run(ctxt *cmd.Context) error {
	client, err := c.newClient(ctxt)
	if err != nil {
		return errgo.Mask(err)
	}
	defer client.Close()
	f := func(path entityPathValue) error {
		return client.DeleteModel(&params.DeleteModel{
			EntityPath: path.EntityPath,
		})
	}
	if c.controller {
		f = func(path entityPathValue) error {
			return client.DeleteController(&params.DeleteController{
				EntityPath: path.EntityPath,
				Force:      c.force,
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
		// We've already printed our error messages.
		return cmd.ErrSilent
	}
	return nil
}
