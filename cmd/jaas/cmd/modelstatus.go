// Copyright 2026 Canonical.

package cmd

import (
	"fmt"

	"github.com/juju/cmd/v3"
	"github.com/juju/gnuflag"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/names/v5"

	apiparams "github.com/canonical/jimm/v3/pkg/api/params"
)

const (
	modelStatusCommandDoc = `
Displays full model status.
`
	modelStatusCommandExample = `
    juju model-status 2cb433a6-04eb-4ec4-9567-90426d20a004
    juju model-status 2cb433a6-04eb-4ec4-9567-90426d20a004 --format yaml
`
)

// NewModelStatusCommand returns a command to display full model status.
func NewModelStatusCommand() cmd.Command {
	cmd := &modelStatusCommand{}
	cmd.SetClientStore(jujuclient.NewFileClientStore())

	return modelcmd.WrapBase(cmd)
}

// modelStatusCommand displays full
// model status.
type modelStatusCommand struct {
	jaasCommandBase
	out cmd.Output

	modelUUID string
}

func (c *modelStatusCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "model-status",
		Args:     "<model uuid>",
		Purpose:  "Displays full model status",
		Doc:      modelStatusCommandDoc,
		Examples: modelStatusCommandExample,
	})
}

// SetFlags implements Command.SetFlags.
func (c *modelStatusCommand) SetFlags(f *gnuflag.FlagSet) {
	c.CommandBase.SetFlags(f)
	c.out.AddFlags(f, "yaml", map[string]cmd.Formatter{
		"yaml": cmd.FormatYaml,
		"json": cmd.FormatJson,
	})
}

// Init implements the cmd.Command interface.
func (c *modelStatusCommand) Init(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("missing model uuid")
	}
	c.modelUUID, args = args[0], args[1:]
	if len(args) > 0 {
		return fmt.Errorf("unknown arguments")
	}
	return nil
}

// Run implements Command.Run.
func (c *modelStatusCommand) Run(ctxt *cmd.Context) error {
	client, err := c.getJIMMAPI()
	if err != nil {
		return err
	}
	defer client.Close()

	modelTag := names.NewModelTag(c.modelUUID)
	status, err := client.FullModelStatus(&apiparams.FullModelStatusRequest{
		ModelTag: modelTag.String(),
	})
	if err != nil {
		return err
	}

	err = c.out.Write(ctxt, status)
	if err != nil {
		return err
	}
	return nil
}
