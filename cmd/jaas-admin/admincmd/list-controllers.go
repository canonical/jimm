// Copyright 2015 Canonical Ltd.

package admincmd

import (
	"fmt"

	"github.com/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"gopkg.in/errgo.v1"

	"github.com/CanonicalLtd/jem/params"
)

type listControllersCommand struct {
	commandBase
}

func newListControllersCommand() cmd.Command {
	return modelcmd.WrapBase(&listControllersCommand{})
}

var listControllersDoc = `
The list-controllers command lists available controllers.
`

func (c *listControllersCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "list-controllers",
		Purpose: "list controllers",
		Doc:     listControllersDoc,
	}
}

func (c *listControllersCommand) Init(args []string) error {
	if len(args) != 0 {
		return errgo.Newf("got %d arguments, want none", len(args))
	}
	return nil
}

func (c *listControllersCommand) Run(ctxt *cmd.Context) error {
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
