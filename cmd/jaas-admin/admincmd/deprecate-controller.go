// Copyright 2018 Canonical Ltd.

package admincmd

import (
	"github.com/juju/cmd"
	"github.com/juju/gnuflag"
	"gopkg.in/errgo.v1"

	"github.com/CanonicalLtd/jimm/params"
)

type deprecateControllerCommand struct {
	*commandBase

	path  entityPathValue
	unset bool
}

func newDeprecateControllerCommand(c *commandBase) cmd.Command {
	return &deprecateControllerCommand{
		commandBase: c,
	}
}

var deprecateControllerDoc = `
The deprecate-controller command marks a controller
as deprecated. New models will not be created on
deprecated controllers.

Deprecation status can be reset by using the --unset flag.
`

func (c *deprecateControllerCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "deprecate-controller",
		Args:    "<user>/<controllername>",
		Purpose: "deprecate a controller for adding new models",
		Doc:     deprecateControllerDoc,
	}
}

func (c *deprecateControllerCommand) SetFlags(f *gnuflag.FlagSet) {
	f.BoolVar(&c.unset, "unset", false, "Undeprecate controller")
}

func (c *deprecateControllerCommand) Init(args []string) error {
	// Validate and store the entity reference.
	if len(args) == 0 {
		return errgo.Newf("no controller specified")
	}
	if len(args) > 1 {
		return errgo.Newf("too many arguments")
	}
	if err := c.path.Set(args[0]); err != nil {
		return errgo.Mask(err)
	}
	return nil
}

func (c *deprecateControllerCommand) Run(ctxt *cmd.Context) error {
	ctx, cancel := wrapContext(ctxt)
	defer cancel()

	client, err := c.newClient(ctxt)
	if err != nil {
		return errgo.Mask(err)
	}
	defer client.Close()
	err = client.SetControllerDeprecated(ctx, &params.SetControllerDeprecated{
		EntityPath: c.path.EntityPath,
		Body: params.DeprecatedBody{
			Deprecated: !c.unset,
		},
	})
	if err != nil {
		return errgo.Mask(err)
	}
	return nil
}
