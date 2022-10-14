// Copyright 2015 Canonical Ltd.

package admincmd

import (
	"fmt"

	"github.com/juju/cmd"
	"gopkg.in/errgo.v1"

	"github.com/CanonicalLtd/jimm/params"
)

type controllersCommand struct {
	*commandBase
}

func newControllersCommand(c *commandBase) cmd.Command {
	return &controllersCommand{
		commandBase: c,
	}
}

var controllersDoc = `
The controllers command lists available controllers.
`

func (c *controllersCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "controllers",
		Purpose: "list controllers",
		Doc:     controllersDoc,
	}
}

func (c *controllersCommand) Init(args []string) error {
	if len(args) != 0 {
		return errgo.Newf("got %d arguments, want none", len(args))
	}
	return nil
}

func (c *controllersCommand) Run(ctxt *cmd.Context) error {
	ctx, cancel := wrapContext(ctxt)
	defer cancel()

	client, err := c.newClient(ctxt)
	if err != nil {
		return errgo.Mask(err)
	}
	defer client.Close()
	resp, err := client.ListController(ctx, &params.ListController{})
	if err != nil {
		return errgo.Mask(err)
	}
	for _, e := range resp.Controllers {
		fmt.Fprintf(ctxt.Stdout, "%s\n", e.Path)
	}
	return nil
}
