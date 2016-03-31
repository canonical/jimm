// Copyright 2015 Canonical Ltd.

package jemcmd

import (
	"fmt"

	"github.com/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"gopkg.in/errgo.v1"

	"github.com/CanonicalLtd/jem/params"
)

type listServersCommand struct {
	commandBase
}

func newListServersCommand() cmd.Command {
	return modelcmd.WrapBase(&listServersCommand{})
}

var listServersDoc = `
The list-controllers command lists available controllers.
`

func (c *listServersCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "list-controllers",
		Purpose: "list controllers",
		Doc:     listServersDoc,
	}
}

func (c *listServersCommand) Init(args []string) error {
	if len(args) != 0 {
		return errgo.Newf("got %d arguments, want none", len(args))
	}
	return nil
}

func (c *listServersCommand) Run(ctxt *cmd.Context) error {
	client, err := c.newClient(ctxt)
	if err != nil {
		return errgo.Mask(err)
	}
	resp, err := client.ListController(&params.ListController{})
	if err != nil {
		return errgo.Mask(err)
	}
	for _, e := range resp.Controllers {
		fmt.Fprintf(ctxt.Stdout, "%s\n", e.Path)
	}
	return nil
}
