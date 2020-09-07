// Copyright 2015 Canonical Ltd.

package admincmd

import (
	"fmt"

	"github.com/juju/cmd"
	"github.com/juju/gnuflag"
	"gopkg.in/errgo.v1"

	"github.com/CanonicalLtd/jimm/params"
)

type modelsCommand struct {
	*commandBase

	all bool
}

func newModelsCommand(c *commandBase) cmd.Command {
	return &modelsCommand{
		commandBase: c,
	}
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
	f.BoolVar(&c.all, "all", false, "list all models in the system (admin only)")
}

func (c *modelsCommand) Init(args []string) error {
	if len(args) != 0 {
		return errgo.Newf("got %d arguments, want none", len(args))
	}
	return nil
}

func (c *modelsCommand) Run(ctxt *cmd.Context) error {
	ctx, cancel := wrapContext(ctxt)
	defer cancel()

	client, err := c.newClient(ctxt)
	if err != nil {
		return errgo.Mask(err)
	}
	defer client.Close()
	resp, err := client.ListModels(ctx, &params.ListModels{
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
