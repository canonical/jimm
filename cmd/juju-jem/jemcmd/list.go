// Copyright 2015 Canonical Ltd.

package jemcmd

import (
	"fmt"

	"github.com/juju/cmd"
	"gopkg.in/errgo.v1"

	"github.com/CanonicalLtd/jem/params"
)

type listCommand struct {
	commandBase
}

var listDoc = `
The list command lists available environments.
`

func (c *listCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "list",
		Purpose: "list environments",
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
	client, err := c.newClient()
	if err != nil {
		return errgo.Mask(err)
	}
	defer client.Close()
	resp, err := client.ListEnvironments(&params.ListEnvironments{})
	if err != nil {
		return errgo.Mask(err)
	}
	for _, e := range resp.Environments {
		fmt.Fprintf(ctxt.Stdout, "%s\n", e.Path)
	}
	return nil
}
