// Copyright 2016 Canonical Ltd.

package jemcmd

import (
	"github.com/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/utils/keyvalues"
	"gopkg.in/errgo.v1"

	"github.com/CanonicalLtd/jem/params"
)

type setCommand struct {
	commandBase

	controllerPath entityPathValue
	attributes     map[string]string
}

func newSetCommand() cmd.Command {
	return modelcmd.WrapBase(&setCommand{})
}

var setDoc = `
The set command sets information about the location of a
controller such as its associated cloud and region.
`

func (c *setCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "set",
		Args:    "<user>/<envname> key=value [key=value]...",
		Purpose: "Set information location on a controller.",
		Doc:     setDoc,
	}
}

func (c *setCommand) Init(args []string) error {
	if len(args) < 2 {
		return errgo.Newf("got %d arguments, want at least 2", len(args))
	}
	if err := c.controllerPath.Set(args[0]); err != nil {
		return errgo.Mask(err)
	}
	fields, err := keyvalues.Parse(args[1:], false)
	if err != nil {
		return errgo.Notef(err, "invalid set arguments")
	}
	if len(fields) == 0 {
		return errgo.New("no set arguments provided")
	}
	c.attributes = fields

	return nil
}

func (c *setCommand) Run(ctxt *cmd.Context) error {
	client, err := c.newClient(ctxt)
	if err != nil {
		return errgo.Mask(err)
	}
	if err := client.SetControllerLocation(&params.SetControllerLocation{
		EntityPath: c.controllerPath.EntityPath,
		Location: params.ControllerLocation{
			Location: c.attributes,
		},
	}); err != nil {
		return errgo.Mask(err)
	}
	return nil
}
