// Copyright 2021 Canonical Ltd.

package cmd

import (
	"github.com/juju/cmd/v3"
	jujuapi "github.com/juju/juju/api"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/names/v4"

	"github.com/CanonicalLtd/jimm/api"
	apiparams "github.com/CanonicalLtd/jimm/api/params"
	"github.com/CanonicalLtd/jimm/internal/errors"
)

const importModelCommandDoc = `
	import-model imports a model running on a controller to jimm.

	Example:
		jimmctl import-model <controller name> <model-uuid>
`

// NewImportModelCommand returns a command to import a model.
func NewImportModelCommand() cmd.Command {
	cmd := &importModelCommand{
		store: jujuclient.NewFileClientStore(),
	}

	return modelcmd.WrapBase(cmd)
}

// importModelCommand imports a model.
type importModelCommand struct {
	modelcmd.ControllerCommandBase
	store    jujuclient.ClientStore
	dialOpts *jujuapi.DialOpts

	req apiparams.ImportModelRequest
}

// Info implements the cmd.Command interface.
func (c *importModelCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "import-model",
		Args:    "<controller name> <model uuid>",
		Purpose: "Import a model to jimm",
		Doc:     importModelCommandDoc,
	})
}

// Init implements the cmd.Command interface.
func (c *importModelCommand) Init(args []string) error {
	switch len(args) {
	case 0:
		return errors.E("controller not specified")
	case 1:
		return errors.E("model uuid not specified")
	default:
		return errors.E("too many args")
	case 2:
	}

	c.req.Controller = args[0]
	if !names.IsValidModel(args[1]) {
		return errors.E("invalid model uuid")
	}
	c.req.ModelTag = names.NewModelTag(args[1]).String()
	return nil
}

// Run implements Command.Run.
func (c *importModelCommand) Run(ctxt *cmd.Context) error {
	currentController, err := c.store.CurrentController()
	if err != nil {
		return errors.E(err, "could not determine controller")
	}

	apiCaller, err := c.NewAPIRootWithDialOpts(c.store, currentController, "", c.dialOpts)
	if err != nil {
		return err
	}

	client := api.NewClient(apiCaller)
	if err := client.ImportModel(&c.req); err != nil {
		return errors.E(err)
	}
	return nil
}
