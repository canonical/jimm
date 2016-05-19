// Copyright 2015 Canonical Ltd.

package modelcmd

import (
	"fmt"

	"github.com/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"gopkg.in/errgo.v1"

	"github.com/CanonicalLtd/jem/params"
)

type listCommand struct {
	commandBase
}

func newListCommand() cmd.Command {
	return modelcmd.WrapBase(&listCommand{})
}

var listDoc = `
The list command lists available models.
`

func (c *listCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "list",
		Purpose: "list models",
		Doc:     listDoc,
	}
}

func (c *listCommand) Init(args []string) error {
	if len(args) != 0 {
		return errgo.Newf("got %d arguments, want none", len(args))
	}
	return nil
}

func (c *listCommand) Run(ctxt *cmd.Context) error {
	client, err := c.newClient(ctxt)
	if err != nil {
		return errgo.Mask(err)
	}
	resp, err := client.ListModels(&params.ListModels{})
	if err != nil {
		return errgo.Mask(err)
	}
	for _, e := range resp.Models {
		fmt.Fprintf(ctxt.Stdout, "%s\n", e.Path)
	}
	return nil
}
