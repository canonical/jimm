// Copyright 2015 Canonical Ltd.

package modelcmd

import (
	"os"

	"github.com/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"gopkg.in/errgo.v1"
	"gopkg.in/juju/environschema.v1"
	"launchpad.net/gnuflag"

	"github.com/CanonicalLtd/jem/params"
)

type generateCommand struct {
	commandBase

	srvPath entityPathValue
	attrs   map[string]string
	output  string
}

func newGenerateCommand() cmd.Command {
	return modelcmd.WrapBase(&generateCommand{})
}

var generateDoc = `
The generate command generates a YAML file for use as a
model configuration. By default it writes the data to standard
output, with attributes set to any default values found in the
model.

The specified controller attributes are used as the basis for the
configuration file.
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
	f.StringVar(&c.output, "output", "", "")
	f.StringVar(&c.output, "o", "", "file name to write generated data (default standard output)")
}

func (c *generateCommand) Init(args []string) error {
	if len(args) != 0 {
		return errgo.Newf("arguments provided but none expected")
	}
	if c.srvPath.EntityPath.IsZero() {
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
	scCtxt := schemaContext{
		providerType: controllerInfo.ProviderType,
		cmdContext:   ctxt,
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
	// Delete any juju-only attributes.
	for name, field := range rootSchema {
		if field.Group == environschema.JujuGroup {
			delete(rootSchema, name)
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
