// Copyright 2015 Canonical Ltd.

package admincmd

import (
	"github.com/juju/cmd"
	"github.com/juju/gnuflag"
	"gopkg.in/errgo.v1"

	"github.com/CanonicalLtd/jimm/params"
)

type removeCommand struct {
	*commandBase

	paths []entityPathValue
	force bool
}

func newRemoveCommand(c *commandBase) cmd.Command {
	return &removeCommand{
		commandBase: c,
	}
}

var removeDoc = `
The remove command removes controllers.
`

func (c *removeCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "remove",
		Args:    "[<user>/<name>...]",
		Purpose: "remove controllers",
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
	c.commandBase.SetFlags(f)
	f.BoolVar(&c.force, "f", false, "force removal of live controller")
	f.BoolVar(&c.force, "force", false, "")
}

func (c *removeCommand) Run(ctxt *cmd.Context) error {
	ctx, cancel := wrapContext(ctxt)
	defer cancel()

	client, err := c.newClient(ctxt)
	if err != nil {
		return errgo.Mask(err)
	}
	defer client.Close()

	var failed bool
	for _, path := range c.paths {
		ctxt.Verbosef("removing %s", path)
		err := client.DeleteController(ctx, &params.DeleteController{
			EntityPath: path.EntityPath,
			Force:      c.force,
		})
		if err != nil {
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
