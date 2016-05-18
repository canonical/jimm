// Copyright 2015-2016 Canonical Ltd.

package modelcmd

import (
	"io/ioutil"

	"github.com/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/utils/keyvalues"
	"gopkg.in/errgo.v1"
	"gopkg.in/yaml.v2"
	"launchpad.net/gnuflag"

	"github.com/CanonicalLtd/jem/params"
)

type createTemplateCommand struct {
	commandBase

	templatePath entityPathValue
	srvPath      entityPathValue
	attrs        map[string]string
	configFile   string
}

func newCreateTemplateCommand() cmd.Command {
	return modelcmd.WrapBase(&createTemplateCommand{})
}

var createTemplateDoc = `
The create-template command adds a template for a model to the managing
server.  A template holds some of the configuration attributes required by a
controller, and may be used when creating a new model.  Secret attributes may
be set but not retrieved.

The <user>/<name> argument specifies the name that will be given to the
template inside the managing server.  Note that this exists in a different name space
from model and controllers.
`

func (c *createTemplateCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "create-template",
		Args:    "<user>/<name> attr=val ...",
		Purpose: "Add a template to the managing server.",
		Doc:     createTemplateDoc,
	}
}

func (c *createTemplateCommand) SetFlags(f *gnuflag.FlagSet) {
	c.commandBase.SetFlags(f)
	f.Var(&c.srvPath, "c", "controller to use as the schema for the template")
	f.Var(&c.srvPath, "controller", "")
	f.StringVar(&c.configFile, "config", "", "YAML config file containing template configuration")
}

func (c *createTemplateCommand) Init(args []string) error {
	if len(args) < 1 {
		return errgo.Newf("got %d arguments, want at least 1", len(args))
	}
	if err := c.templatePath.Set(args[0]); err != nil {
		return errgo.Mask(err)
	}
	if c.srvPath.EntityPath.IsZero() {
		return errgo.Newf("--controller flag required but not provided")
	}
	if c.configFile != "" {
		if len(args) > 1 {
			return errgo.Newf("--config cannot be specified with attr=value key pairs")
		}
		return nil
	}
	attrs, err := keyvalues.Parse(args[1:], true)
	if err != nil {
		return errgo.Mask(err)
	}
	c.attrs = attrs
	return nil
}

func (c *createTemplateCommand) Run(ctxt *cmd.Context) error {
	client, err := c.newClient(ctxt)
	if err != nil {
		return errgo.Mask(err)
	}

	config := make(map[string]interface{})
	if c.configFile != "" {
		data, err := ioutil.ReadFile(c.configFile)
		if err != nil {
			return errgo.Notef(err, "cannot read configuration file")
		}
		if err := yaml.Unmarshal(data, &config); err != nil {
			return errgo.Notef(err, "cannot unmarshal %q", c.configFile)
		}
	} else {
		for name, val := range c.attrs {
			config[name] = val
		}
	}
	// TODO support creating template with location.
	if err := client.AddTemplate(&params.AddTemplate{
		EntityPath: c.templatePath.EntityPath,
		Info: params.AddTemplateInfo{
			Controller: &c.srvPath.EntityPath,
			Config:     config,
		},
	}); err != nil {
		return errgo.Notef(err, "cannot add template")
	}
	return nil
}
