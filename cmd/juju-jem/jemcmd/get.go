// Copyright 2015 Canonical Ltd.

package jemcmd

import (
	"github.com/juju/cmd"
	"gopkg.in/errgo.v1"
	"launchpad.net/gnuflag"

	"github.com/CanonicalLtd/jem/params"
)

type getCommand struct {
	commandBase

	envPath   entityPathValue
	localName string
	user      string
}

var getDoc = `
The get command gets information about a JEM environment
and writes it to a local .jenv file.
`

func (c *getCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "get",
		Args:    "<user>/<envname>",
		Purpose: "Make a JEM environment available to Juju",
		Doc:     getDoc,
	}
}

func (c *getCommand) SetFlags(f *gnuflag.FlagSet) {
	c.commandBase.SetFlags(f)
	f.StringVar(&c.localName, "local", "", "local name for environment (as used for juju switch). Defaults to <envname>")
	f.StringVar(&c.user, "u", "", "user name to use when accessing the environment (defaults to user name created for environment)")
	f.StringVar(&c.user, "user", "", "")
}

func (c *getCommand) Init(args []string) error {
	if len(args) != 1 {
		return errgo.Newf("got %d arguments, want 1", len(args))
	}
	if err := c.envPath.Set(args[0]); err != nil {
		return errgo.Mask(err)
	}
	if c.localName == "" {
		c.localName = string(c.envPath.Name)
	}
	return nil
}

func (c *getCommand) Run(ctxt *cmd.Context) error {
	client, err := c.newClient()
	if err != nil {
		return errgo.Mask(err)
	}
	defer client.Close()

	return writeEnvironment(c.localName, func() (*params.EnvironmentResponse, error) {
		resp, err := client.GetEnvironment(&params.GetEnvironment{
			EntityPath: c.envPath.EntityPath,
		})
		if err != nil {
			return nil, errgo.Notef(err, "cannot get environment info")
		}
		if c.user != "" {
			resp.User = c.user
		}
		return resp, nil
	})
}
