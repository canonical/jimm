// Copyright 2015-2016 Canonical Ltd.

package modelcmd

import (
	"github.com/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/utils/keyvalues"
	"gopkg.in/errgo.v1"
	"launchpad.net/gnuflag"

	"github.com/CanonicalLtd/jem/params"
)

type addControllerCommand struct {
	commandBase

	modelName  string
	modelPath  entityPathValue
	attributes map[string]string
	public     bool
}

func newAddControllerCommand() cmd.Command {
	return modelcmd.WrapBase(&addControllerCommand{})
}

var addControllerDoc = `
The add-controller command adds an existing Juju controller to the managing server.
It takes the information from the data stored locally by juju (the
current model by default).

The <user>/<name> argument specifies the name
that will be given to the controller inside the managing server.
This will also be added as a model, so the
commands which refer to a model
can also use the controller name.
Some key value pair could be specify like cloud=aws be be set directly
on the location information of the controller.

The location of the controller can be set by providing a set of key=value
pairs as additional arguments, for example:
 
     jaas model add-controller --public alice/mycontroller cloud=aws region=us-east-1

Without the --public flag, the location will not cause the controller
to be considered a candidate for selection by location - the location
is for annotation only in this case (this might change).
`

func (c *addControllerCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "add-controller",
		Args:    "<user>/<name> key=value [key=value...]",
		Purpose: "Add a controller to the managing server.",
		Doc:     addControllerDoc,
	}
}

func (c *addControllerCommand) SetFlags(f *gnuflag.FlagSet) {
	c.commandBase.SetFlags(f)
	f.StringVar(&c.modelName, "m", "", "model to add")
	f.StringVar(&c.modelName, "model", "", "")
	f.BoolVar(&c.public, "public", false, "whether it will be part of the public pool of controllers")
}

func (c *addControllerCommand) Init(args []string) error {
	if len(args) < 1 {
		return errgo.Newf("got %d arguments, want 1", len(args))
	}
	if err := c.modelPath.Set(args[0]); err != nil {
		return errgo.Mask(err)
	}
	attrs, err := keyvalues.Parse(args[1:], false)
	if err != nil {
		return errgo.Mask(err)
	}
	c.attributes = attrs
	return nil
}

func (c *addControllerCommand) Run(ctxt *cmd.Context) error {
	client, err := c.newClient(ctxt)
	if err != nil {
		return errgo.Mask(err)
	}
	info, err := getModelInfo(c.modelName)
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

	logger.Infof("adding controller, user %s, name %s", c.modelPath.EntityPath.User, c.modelPath.EntityPath.Name)
	if err := client.AddController(&params.AddController{
		EntityPath: c.modelPath.EntityPath,
		Info: params.ControllerInfo{
			HostPorts:      hostnames,
			CACert:         info.controller.CACert,
			ControllerUUID: info.controller.ControllerUUID,
			User:           info.account.User,
			Password:       info.account.Password,
			Location:       c.attributes,
			Public:         c.public,
		},
	}); err != nil {
		return errgo.Notef(err, "cannot add controller")
	}
	return nil
}

type modelInfo struct {
	model      *jujuclient.ModelDetails
	controller *jujuclient.ControllerDetails
	account    *jujuclient.AccountDetails
}

func getModelInfo(modelName string) (*modelInfo, error) {
	store := jujuclient.NewFileClientStore()
	var err error
	var controllerName string
	if modelName == "" {
		modelName, err = modelcmd.GetCurrentModel(store)
		if err != nil {
			return nil, errgo.Mask(err)
		}
	}
	controllerName, modelName = modelcmd.SplitModelName(modelName)
	if controllerName == "" {
		controllerName, err = modelcmd.ReadCurrentController()
		if err != nil {
			return nil, errgo.Mask(err)
		}
	}
	accountName, err := store.CurrentAccount(controllerName)
	if err != nil {
		return nil, errgo.Mask(err)
	}

	var info modelInfo
	info.model, err = store.ModelByName(controllerName, accountName, modelName)
	if err != nil {
		return nil, errgo.Mask(err)
	}
	info.controller, err = store.ControllerByName(controllerName)
	if err != nil {
		return nil, errgo.Mask(err)
	}
	info.account, err = store.AccountByName(controllerName, accountName)
	if err != nil {
		return nil, errgo.Mask(err)
	}
	return &info, nil
}
