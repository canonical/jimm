// Copyright 2026 Canonical.

package cmd

import (
	"fmt"
	"io"

	"github.com/juju/cmd/v3"
	"github.com/juju/gnuflag"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/cmd/output"
	"github.com/juju/juju/jujuclient"

	apiparams "github.com/canonical/jimm/v3/pkg/api/params"
)

const (
	showModelCommandDoc = `
Displays information about which controller the specified model is running on.

The model can be specified using either:
  - Model UUID (e.g., "2cb433a6-04eb-4ec4-9567-90426d20a004")
  - Owner and model name (e.g., "alice@canonical.com/my-model")

The output includes the model name, model UUID, controller name, and controller UUID.
`
	showModelCommandExample = `
    jaas show-model 2cb433a6-04eb-4ec4-9567-90426d20a004
    jaas show-model alice@canonical.com/my-model
    jaas show-model alice@canonical.com/my-model --format json
    jaas show-model alice@canonical.com/my-model --format yaml
`
)

// NewShowModelCommand returns a command to display model controller information.
func NewShowModelCommand() cmd.Command {
	cmd := &showModelCommand{}
	cmd.SetClientStore(jujuclient.NewFileClientStore())

	return modelcmd.WrapBase(cmd)
}

// showModelCommand displays information about
// which controller the specified model is running on.
type showModelCommand struct {
	jaasCommandBase
	out cmd.Output

	modelQualifier string
}

func (c *showModelCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "show-model",
		Args:     "<model>",
		Purpose:  "Displays information about a model and its controller",
		Doc:      showModelCommandDoc,
		Examples: showModelCommandExample,
	})
}

// SetFlags implements Command.SetFlags.
func (c *showModelCommand) SetFlags(f *gnuflag.FlagSet) {
	c.CommandBase.SetFlags(f)
	c.out.AddFlags(f, "tabular", map[string]cmd.Formatter{
		"yaml":    cmd.FormatYaml,
		"json":    cmd.FormatJson,
		"tabular": c.formatTabular,
	})
}

// Init implements the cmd.Command interface.
func (c *showModelCommand) Init(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("missing model qualifier")
	}
	c.modelQualifier, args = args[0], args[1:]
	if len(args) > 0 {
		return fmt.Errorf("unknown arguments: %v", args)
	}
	return nil
}

// Run implements Command.Run.
func (c *showModelCommand) Run(ctxt *cmd.Context) error {
	client, err := c.getJIMMAPI()
	if err != nil {
		return err
	}
	defer client.Close()

	info, err := client.ModelControllerInfo(c.modelQualifier)
	if err != nil {
		return err
	}

	err = c.out.Write(ctxt, info)
	if err != nil {
		return err
	}
	return nil
}

// formatTabular formats the model controller info in a tabular format.
func (c *showModelCommand) formatTabular(writer io.Writer, value any) error {
	info, ok := value.(*apiparams.ModelControllerInfo)
	if !ok {
		return fmt.Errorf("expected *apiparams.ModelControllerInfo, got %T", value)
	}

	tw := output.TabWriter(writer)
	w := output.Wrapper{TabWriter: tw}

	w.PrintHeaders(output.EmphasisHighlight.DefaultBold, "Model name", "Model UUID", "Controller name", "Controller UUID")
	w.Println(info.ModelName, info.ModelUUID, info.ControllerName, info.ControllerUUID)
	w.Flush()

	return nil
}
