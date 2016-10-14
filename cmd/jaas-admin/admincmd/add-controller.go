// Copyright 2015-2016 Canonical Ltd.

package admincmd

import (
	"net"

	"github.com/juju/cmd"
	"github.com/juju/gnuflag"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/bootstrap"
	"github.com/juju/juju/jujuclient"
	"gopkg.in/errgo.v1"

	"github.com/CanonicalLtd/jem/params"
)

type addControllerCommand struct {
	commandBase

	publicHostname string
	controllerName string
	controllerPath entityPathValue
}

func newAddControllerCommand() cmd.Command {
	return modelcmd.WrapBase(&addControllerCommand{})
}

var addControllerDoc = `
The add-controller command adds an existing Juju controller to the
managing server.  It takes the information from the data stored locally
by juju (the current model by default).

The <user>/<name> argument specifies the name that will be given to
the controller inside the managing server.

The controller that is added will be made available to all logged
in users.
`

func (c *addControllerCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "add-controller",
		Args:    "<user>/<name>",
		Purpose: "Add a controller to the managing server.",
		Doc:     addControllerDoc,
	}
}

func (c *addControllerCommand) SetFlags(f *gnuflag.FlagSet) {
	c.commandBase.SetFlags(f)
	f.StringVar(&c.controllerName, "c", "", "controller to add")
	f.StringVar(&c.controllerName, "controller", "", "")
	f.StringVar(&c.publicHostname, "public-hostname", "", "public hostname for the controller.")
}

func (c *addControllerCommand) Init(args []string) error {
	if len(args) != 1 {
		return errgo.Newf("got %d arguments, want 1", len(args))
	}
	if err := c.controllerPath.Set(args[0]); err != nil {
		return errgo.Mask(err)
	}
	return nil
}

func (c *addControllerCommand) Run(ctxt *cmd.Context) error {
	client, err := c.newClient(ctxt)
	if err != nil {
		return errgo.Mask(err)
	}
	logger.Debugf("client: %#v\n", client)
	info, err := getControllerInfo(c.controllerName)
	if err != nil {
		return errgo.Mask(err)
	}
	// Use hostnames by preference, as we want the
	// JEM server to do the DNS lookups, but this
	// may not be set, so fall back to Addresses if
	// necessary.
	hostnames := info.controller.APIEndpoints
	if len(hostnames) == 0 {
		hostnames = info.controller.UnresolvedAPIEndpoints
	}
	if c.publicHostname != "" && len(hostnames) > 0 {
		_, port, err := net.SplitHostPort(hostnames[0])
		if err != nil {
			// This should never happen with data written by juju.
			return errgo.Mask(err)
		}
		hostnames = []string{net.JoinHostPort(c.publicHostname, port)}
	}
	logger.Infof("adding controller, user %s, name %s", c.controllerPath.EntityPath.User, c.controllerPath.EntityPath.Name)
	if err := client.AddController(&params.AddController{
		EntityPath: c.controllerPath.EntityPath,
		Info: params.ControllerInfo{
			HostPorts:      hostnames,
			CACert:         info.controller.CACert,
			ControllerUUID: info.controller.ControllerUUID,
			User:           info.account.User,
			Password:       info.account.Password,
			Public:         true,
			Cloud:          params.Cloud(info.controller.Cloud),
			Region:         info.controller.CloudRegion,
		},
	}); err != nil {
		return errgo.Notef(err, "cannot add controller")
	}
	if err := client.SetControllerPerm(&params.SetControllerPerm{
		EntityPath: c.controllerPath.EntityPath,
		ACL: params.ACL{
			Read: []string{"everyone"},
		},
	}); err != nil {
		return errgo.Notef(err, "cannot set controller permissions")
	}
	return nil
}

type controllerInfo struct {
	model      *jujuclient.ModelDetails
	controller *jujuclient.ControllerDetails
	account    *jujuclient.AccountDetails
}

func getControllerInfo(controllerName string) (*controllerInfo, error) {
	store := jujuclient.NewFileClientStore()
	var err error
	if controllerName == "" {
		controllerName, err = store.CurrentController()
		if err != nil {
			return nil, errgo.Mask(err)
		}
	}

	var info controllerInfo
	info.model, err = store.ModelByName(controllerName, environs.AdminUser+"/"+bootstrap.ControllerModelName)
	if err != nil {
		return nil, errgo.Mask(err)
	}
	info.controller, err = store.ControllerByName(controllerName)
	if err != nil {
		return nil, errgo.Mask(err)
	}
	info.account, err = store.AccountDetails(controllerName)
	if err != nil {
		return nil, errgo.Mask(err)
	}
	return &info, nil
}
