// Copyright 2015 Canonical Ltd.

package jemcmd

import (
	"fmt"

	"github.com/juju/cmd"
	"gopkg.in/errgo.v1"

	"github.com/CanonicalLtd/jem/params"
)

type listServersCommand struct {
	commandBase
}

var listServersDoc = `
The list-servers command lists available state servers.
`

func (c *listServersCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "list-servers",
		Purpose: "list state servers",
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
	defer client.Close()
	resp, err := client.ListJES(&params.ListJES{})
	if err != nil {
		return errgo.Mask(err)
	}
	for _, e := range resp.StateServers {
		fmt.Fprintf(ctxt.Stdout, "%s\n", e.Path)
	}
	return nil
}
