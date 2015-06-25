// Copyright 2015 Canonical Ltd.

package jemcmd

import (
	"github.com/juju/cmd"
	"launchpad.net/gnuflag"
)

type addServerCommand struct {
	cmd.CommandBase
}

var addServerDoc = `
The add-server command ... TODO.
`

func (c *addServerCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "add-server",
		Args:    "TODO",
		Purpose: "TODO",
		Doc:     addServerDoc,
	}
}

func (c *addServerCommand) SetFlags(f *gnuflag.FlagSet) {
	// f.StringVar(&c.value, "flagname", "default", "description")
}

func (c *addServerCommand) Init(args []string) error {
	// TODO
	return nil
}

func (c *addServerCommand) Run(ctxt *cmd.Context) error {
	// TODO
	return nil
}
