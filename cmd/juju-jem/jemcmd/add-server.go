// Copyright 2015 Canonical Ltd.

package jemcmd

import (
	"github.com/juju/cmd"
	"github.com/juju/juju/cmd/envcmd"
	"gopkg.in/errgo.v1"
	"launchpad.net/gnuflag"

	"github.com/CanonicalLtd/jem/params"
)

type addServerCommand struct {
	commandBase

	envName string
	envPath entityPathValue
}

func newAddServerCommand() cmd.Command {
	return envcmd.WrapBase(&addServerCommand{})
}

var addServerDoc = `
The add-controller command adds an existing Juju controller (formerly known as
state server) to the JEM. It takes the information from the
data stored locally by juju (the current model by default).

The <user>/<name> argument specifies the name
that will be given to the controller inside JEM.
This will also be added as a model, so the
JEM commands which refer to a model
can also use the controller name.
`

func (c *addServerCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "add-controller",
		Args:    "<user>/<name>",
		Purpose: "Add a controller to JEM.",
		Doc:     addServerDoc,
	}
}

func (c *addServerCommand) SetFlags(f *gnuflag.FlagSet) {
	c.commandBase.SetFlags(f)
	f.StringVar(&c.envName, "m", "", "model to add")
	f.StringVar(&c.envName, "model", "", "")
}

func (c *addServerCommand) Init(args []string) error {
	if len(args) != 1 {
		return errgo.Newf("got %d arguments, want 1", len(args))
	}
	if err := c.envPath.Set(args[0]); err != nil {
		return errgo.Mask(err)
	}
	return nil
}

func (c *addServerCommand) Run(ctxt *cmd.Context) error {
	client, err := c.newClient(ctxt)
	if err != nil {
		return errgo.Mask(err)
	}
	info, err := environInfo(c.envName)
	if err != nil {
		return errgo.Mask(err)
	}
	ep := info.APIEndpoint()
	creds := info.APICredentials()
	// Use hostnames by preference, as we want the
	// JEM server to do the DNS lookups, but this
	// may not be set, so fall back to Addresses if
	// necessary.
	hostnames := ep.Hostnames
	if len(hostnames) == 0 {
		hostnames = ep.Addresses
	}

	logger.Infof("adding JES, user %s, name %s")
	if err := client.AddJES(&params.AddJES{
		EntityPath: c.envPath.EntityPath,
		Info: params.ServerInfo{
			HostPorts:   hostnames,
			CACert:      ep.CACert,
			EnvironUUID: ep.EnvironUUID,
			User:        creds.User,
			Password:    creds.Password,
		},
	}); err != nil {
		return errgo.Notef(err, "cannot add controller")
	}
	return nil
}
