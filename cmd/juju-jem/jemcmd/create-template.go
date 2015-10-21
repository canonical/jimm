// Copyright 2015 Canonical Ltd.

package jemcmd

import (
	"github.com/juju/cmd"
	"github.com/juju/utils/keyvalues"
	"gopkg.in/errgo.v1"
	"gopkg.in/juju/environschema.v1/form"
	"launchpad.net/gnuflag"

	"github.com/CanonicalLtd/jem/params"
)

type createTemplateCommand struct {
	commandBase

	templatePath entityPathValue
	srvPath      entityPathValue
	attrs        map[string]string
	interactive  bool
}

var createTemplateDoc = `
The create-template command adds a template for an environment to the JEM.
A template holds some of the configuration attributes required by
a state server, and may be used when creating a new environment.
Secret attributes may be set but not retrieved.

The <user>/<name> argument specifies the name that will be given to the
template inside JEM.  Note that this exists in a different name space
from environments and state servers.
`

func (c *createTemplateCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "create-template",
		Args:    "<user>/<name> attr=val ...",
		Purpose: "Add a template to JEM.",
		Doc:     createTemplateDoc,
	}
}

func (c *createTemplateCommand) SetFlags(f *gnuflag.FlagSet) {
	c.commandBase.SetFlags(f)
	f.Var(&c.srvPath, "s", "state server to use as the schema for the template")
	f.Var(&c.srvPath, "server", "")
	f.BoolVar(&c.interactive, "i", false, "interactively prompt for configuration attributes")
	f.BoolVar(&c.interactive, "interactive", false, "")
	// TODO
	//f.BoolVar(&c.useDefaults, "default", false, "use environment variables to set default values")
	// This would call setConfigDefaults on the configuration
	// attributes so that it's easy to make a template with
	// the current defaults.
}

func (c *createTemplateCommand) Init(args []string) error {
	if len(args) < 1 {
		return errgo.Newf("got %d arguments, want at least 1", len(args))
	}
	if err := c.templatePath.Set(args[0]); err != nil {
		return errgo.Mask(err)
	}
	if c.srvPath.EntityPath == (params.EntityPath{}) {
		return errgo.Newf("--server flag required but not provided")
	}
	if c.interactive {
		// TODO allow attributes to specify default values (or just omit)
		// in interactive mode?
		if len(args) > 1 {
			return errgo.Newf("attributes cannot be specified in interactive mode")
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

var ioFillerFill = form.IOFiller.Fill

func (c *createTemplateCommand) Run(ctxt *cmd.Context) error {
	client, err := c.newClient(ctxt)
	if err != nil {
		return errgo.Mask(err)
	}
	defer client.Close()

	var config map[string]interface{}
	if c.interactive {
		jesInfo, err := client.GetJES(&params.GetJES{
			EntityPath: c.srvPath.EntityPath,
		})
		if err != nil {
			return errgo.Notef(err, "cannot get state server")
		}
		defaultsCtxt := schemaContext{
			providerType: jesInfo.ProviderType,
		}
		filler := form.IOFiller{
			In:               ctxt.Stdin,
			Out:              ctxt.Stdout,
			ShowDescriptions: true,
			GetDefault:       defaultsCtxt.getDefault,
		}
		// All attributes are optional in a template.
		for name, field := range jesInfo.Schema {
			field.Mandatory = false
			jesInfo.Schema[name] = field
		}
		config, err = ioFillerFill(filler, form.Form{
			Fields: jesInfo.Schema,
		})
		if err != nil {
			return errgo.Mask(err)
		}
	} else {
		// TODO GetJES to find the schema when implementing
		// the --default flag.
		config = make(map[string]interface{})
		for name, val := range c.attrs {
			config[name] = val
		}
	}
	if err := client.AddTemplate(&params.AddTemplate{
		EntityPath: c.templatePath.EntityPath,
		Info: params.AddTemplateInfo{
			StateServer: c.srvPath.EntityPath,
			Config:      config,
		},
	}); err != nil {
		return errgo.Notef(err, "cannot add template")
	}
	return nil
}
