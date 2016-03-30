// Copyright 2015 Canonical Ltd.

package jemcmd

import (
	"github.com/juju/cmd"
	"github.com/juju/juju/cmd/envcmd"
	"gopkg.in/errgo.v1"
	"launchpad.net/gnuflag"

	"github.com/CanonicalLtd/jem/params"
)

type addControllerCommand struct {
	commandBase

	modelName string
	modelPath entityPathValue
}

func newAddControllerCommand() cmd.Command {
	return envcmd.WrapBase(&addControllerCommand{})
}

var addControllerDoc = `
The add-controller command adds an existing Juju controller to the JEM.
It takes the information from the data stored locally by juju (the
current model by default).

The <user>/<name> argument specifies the name
that will be given to the controller inside JEM.
This will also be added as a model, so the
JEM commands which refer to a model
can also use the controller name.
`

func (c *addControllerCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "add-controller",
		Args:    "<user>/<name>",
		Purpose: "Add a controller to JEM.",
		Doc:     addControllerDoc,
	}
}

func (c *addControllerCommand) SetFlags(f *gnuflag.FlagSet) {
	c.commandBase.SetFlags(f)
	f.StringVar(&c.modelName, "m", "", "model to add")
	f.StringVar(&c.modelName, "model", "", "")
}

func (c *addControllerCommand) Init(args []string) error {
	if len(args) != 1 {
		return errgo.Newf("got %d arguments, want 1", len(args))
	}
	if err := c.modelPath.Set(args[0]); err != nil {
		return errgo.Mask(err)
	}
	return nil
}

func (c *addControllerCommand) Run(ctxt *cmd.Context) error {
	client, err := c.newClient(ctxt)
	if err != nil {
		return errgo.Mask(err)
	}
	info, err := modelInfo(c.modelName)
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

	logger.Infof("adding controller, user %s, name %s", c.modelPath.EntityPath.User, c.modelPath.EntityPath.Name)
	if err := client.AddController(&params.AddController{
		EntityPath: c.modelPath.EntityPath,
		Info: params.ControllerInfo{
			HostPorts: hostnames,
			CACert:    ep.CACert,
			ModelUUID: ep.EnvironUUID,
			User:      creds.User,
			Password:  creds.Password,
		},
	}); err != nil {
		return errgo.Notef(err, "cannot add controller")
	}
	return nil
}
