// Copyright 2015 Canonical Ltd.

package jemcmd

import (
	"github.com/juju/cmd"
	"github.com/juju/juju/cmd/envcmd"
	"gopkg.in/errgo.v1"
	"launchpad.net/gnuflag"

	"github.com/CanonicalLtd/jem/params"
)

type getCommand struct {
	commandBase

	modelPath entityPathValue
	localName string
	user      string
}

func newGetCommand() cmd.Command {
	return envcmd.WrapBase(&getCommand{})
}

var getDoc = `
The get command gets information about a JEM model
and writes it to a local .jenv file.
`

func (c *getCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "get",
		Args:    "<user>/<envname>",
		Purpose: "Make a JEM model available to Juju",
		Doc:     getDoc,
	}
}

func (c *getCommand) SetFlags(f *gnuflag.FlagSet) {
	c.commandBase.SetFlags(f)
	f.StringVar(&c.localName, "local", "", "local name for model (as used for juju switch). Defaults to <envname>")
	f.StringVar(&c.user, "u", "", "user name to use when accessing the model (defaults to user name created for model)")
	f.StringVar(&c.user, "user", "", "")
}

func (c *getCommand) Init(args []string) error {
	if len(args) != 1 {
		return errgo.Newf("got %d arguments, want 1", len(args))
	}
	if err := c.modelPath.Set(args[0]); err != nil {
		return errgo.Mask(err)
	}
	if c.localName == "" {
		c.localName = string(c.modelPath.Name)
	}
	return nil
}

func (c *getCommand) Run(ctxt *cmd.Context) error {
	client, err := c.newClient(ctxt)
	if err != nil {
		return errgo.Mask(err)
	}

	return writeModel(c.localName, func() (*params.ModelResponse, error) {
		resp, err := client.GetModel(&params.GetModel{
			EntityPath: c.modelPath.EntityPath,
		})
		if err != nil {
			return nil, errgo.Notef(err, "cannot get model info")
		}
		if c.user != "" {
			resp.User = c.user
		}
		return resp, nil
	})
}
