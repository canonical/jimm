// Copyright 2015 Canonical Ltd.

package jemcmd

import (
	"os"

	"github.com/juju/cmd"
	"github.com/juju/juju/cmd/envcmd"
	"gopkg.in/errgo.v1"
	"gopkg.in/juju/environschema.v1"
	"launchpad.net/gnuflag"

	"github.com/CanonicalLtd/jem/params"
)

type generateCommand struct {
	commandBase

	templatePaths entityPathsValue
	srvPath       entityPathValue
	attrs         map[string]string
	output        string
	forTemplate   bool
}

func newGenerateCommand() cmd.Command {
	return envcmd.WrapBase(&generateCommand{})
}

var generateDoc = `
The generate command generates a YAML file for use as a template or
model configuration. By default it writes the data to standard
output, with attributes set to any default values found in the
model.

The specified controller attributes are used as the basis for the
configuration file. If any templates are specified, their attributes
will not be written - the effect is to write configuration data that
is necessary when using the create command with the controller and
those templates.
`

func (c *generateCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "generate",
		Args:    "",
		Purpose: "Generate configuration YAML data",
		Doc:     generateDoc,
	}
}

func (c *generateCommand) SetFlags(f *gnuflag.FlagSet) {
	c.commandBase.SetFlags(f)
	f.Var(&c.srvPath, "c", "controller to use as the schema for the file")
	f.Var(&c.srvPath, "controller", "")
	f.Var(&c.templatePaths, "template", "")
	f.Var(&c.templatePaths, "t", "comma-separated templates to exclude from generated configuration")
	f.StringVar(&c.output, "output", "", "")
	f.StringVar(&c.output, "o", "", "file name to write generated data (default standard output)")
	f.BoolVar(&c.forTemplate, "for-template", false, "")
	f.BoolVar(&c.forTemplate, "T", false, "generate data for template (no mandatory attributes)")
}

func (c *generateCommand) Init(args []string) error {
	if len(args) != 0 {
		return errgo.Newf("arguments provided but none expected")
	}
	if c.srvPath.EntityPath == (params.EntityPath{}) {
		return errgo.Newf("controller must be specified")
	}
	return nil
}

func (c *generateCommand) Run(ctxt *cmd.Context) error {
	client, err := c.newClient(ctxt)
	if err != nil {
		return errgo.Mask(err)
	}
	controllerInfo, err := client.GetController(&params.GetController{
		EntityPath: c.srvPath.EntityPath,
	})
	if err != nil {
		return errgo.Notef(err, "cannot get Controller info")
	}
	rootSchema := controllerInfo.Schema
	templates := make([]*params.TemplateResponse, len(c.templatePaths.paths))
	for i, tp := range c.templatePaths.paths {
		t, err := client.GetTemplate(&params.GetTemplate{
			EntityPath: tp,
		})
		if err != nil {
			return errgo.Notef(err, "cannot get template info")
		}
		templates[i] = t
	}
	scCtxt := schemaContext{
		providerType: controllerInfo.ProviderType,
	}
	if scCtxt.providerType == "local" {
		// We have no way of deciding on the default value
		// for the namespace attribute because we don't
		// have an model name, so prevent
		// it being used to generate a default, which would
		// fail, by adding that attribute to the list
		// of known attributes.
		scCtxt.knownAttrs = map[string]interface{}{
			"namespace": nil,
		}
	}
	for _, t := range templates {
		for name := range t.Config {
			delete(rootSchema, name)
		}
	}
	// Delete any juju-only attributes.
	for name, field := range rootSchema {
		if field.Group == environschema.JujuGroup {
			delete(rootSchema, name)
		} else if c.forTemplate && field.Mandatory {
			// If we're generating for a template, all fields are optional.
			field.Mandatory = false
			rootSchema[name] = field
		}
	}
	attrs, err := scCtxt.generateConfig(rootSchema)
	if err != nil {
		return errgo.Notef(err, "cannot generate default values")
	}
	out := ctxt.Stdout
	if c.output != "" {
		f, err := os.Create(ctxt.AbsPath(c.output))
		if err != nil {
			return errgo.Mask(err)
		}
		defer f.Close()
		out = f
	}
	if err := environschema.SampleYAML(out, 0, attrs, rootSchema); err != nil {
		return errgo.Notef(err, "cannot generate sample configuration")
	}
	return nil
}
