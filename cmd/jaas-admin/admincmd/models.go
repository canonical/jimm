// Copyright 2015 Canonical Ltd.

package admincmd

import (
	"fmt"

	"github.com/juju/cmd"
	"github.com/juju/gnuflag"
	"github.com/juju/juju/cmd/modelcmd"
	"gopkg.in/errgo.v1"

	"github.com/CanonicalLtd/jem/params"
)

type modelsCommand struct {
	commandBase

	all bool
}

func newModelsCommand() cmd.Command {
	return modelcmd.WrapBase(&modelsCommand{})
}

var modelsDoc = `
The models command lists available models.
`

func (c *modelsCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "models",
		Purpose: "list models",
		Doc:     modelsDoc,
	}
}

func (c *modelsCommand) SetFlags(f *gnuflag.FlagSet) {
	c.commandBase.SetFlags(f)
	f.BoolVar(&c.all, "all", false, "list all models in the system (admin only)")
}

func (c *modelsCommand) Init(args []string) error {
	if len(args) != 0 {
		return errgo.Newf("got %d arguments, want none", len(args))
	}
	return nil
}

func (c *modelsCommand) Run(ctxt *cmd.Context) error {
	client, err := c.newClient(ctxt)
	if err != nil {
		return errgo.Mask(err)
	}
	resp, err := client.ListModels(&params.ListModels{
		All: c.all,
	})
	if err != nil {
		return errgo.Mask(err)
	}
	for _, e := range resp.Models {
		fmt.Fprintf(ctxt.Stdout, "%s\n", e.Path)
	}
	return nil
}
