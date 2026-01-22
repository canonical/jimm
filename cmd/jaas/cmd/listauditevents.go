// Copyright 2025 Canonical.

package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/gosuri/uitable"
	"github.com/juju/cmd/v3"
	"github.com/juju/gnuflag"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/jujuclient"

	"github.com/canonical/jimm/v3/pkg/api"
	apiparams "github.com/canonical/jimm/v3/pkg/api/params"
)

const (
	listAuditEventsCommandDoc = `
Returns audit log events.
`
	listAuditEventsCommandExample = `
    juju list-audit-events --after 2020-01-01T15:00:00 --before 2020-01-01T15:00:00 --user-tag user@canonical.com --limit 50
    juju list-audit-events --method CreateModel
    juju audit-events --after 2020-01-01T15:00:00 --format yaml
`
)

// NewListAuditEventsCommand returns a command to list audit events matching
// specified criteria.
func NewListAuditEventsCommand() cmd.Command {
	cmd := &listAuditEventsCommand{
		store: jujuclient.NewFileClientStore(),
	}
	cmd.jimmAPIFunc = cmd.newClient

	return modelcmd.WrapBase(cmd)
}

// listAuditEventsCommand displays full
// model status.
type listAuditEventsCommand struct {
	modelcmd.ControllerCommandBase
	out cmd.Output

	args apiparams.FindAuditEventsRequest

	store       jujuclient.ClientStore
	jimmAPIFunc func() (JIMMAPI, error)
}

func (c *listAuditEventsCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "list-audit-events",
		Purpose:  "Displays audit events",
		Doc:      listAuditEventsCommandDoc,
		Examples: listAuditEventsCommandExample,
		Aliases:  []string{"audit-events"},
	})
}

// SetFlags implements Command.SetFlags.
func (c *listAuditEventsCommand) SetFlags(f *gnuflag.FlagSet) {
	c.CommandBase.SetFlags(f)
	c.out.AddFlags(f, "yaml", map[string]cmd.Formatter{
		"yaml":    cmd.FormatYaml,
		"json":    cmd.FormatJson,
		"tabular": formatTabular,
	})
	f.StringVar(&c.args.After, "after", "", "display events that happened after a specified time, formatted as RFC3339")
	f.StringVar(&c.args.Before, "before", "", "display events that happened before specified time, formatted as RFC3339")
	f.StringVar(&c.args.UserTag, "user-tag", "", "display events performed by authenticated user")
	f.StringVar(&c.args.Method, "method", "", "display events for a specific method call")
	f.StringVar(&c.args.Model, "model", "", "display events for a specific model (model name is controller/model)")
	f.IntVar(&c.args.Offset, "offset", 0, "offset the set of returned audit events")
	f.IntVar(&c.args.Limit, "limit", 0, "limit the maximum number of returned audit events")
	f.BoolVar(&c.args.SortTime, "reverse", false, "reverse the order of logs, showing the most recent first")

}

// Init implements the cmd.Command interface.
func (c *listAuditEventsCommand) Init(args []string) error {
	if len(args) > 0 {
		return errors.New("unknown arguments")
	}
	return nil
}

// Run implements Command.Run.
func (c *listAuditEventsCommand) Run(ctxt *cmd.Context) error {
	if c.jimmAPIFunc == nil {
		c.jimmAPIFunc = c.newClient
	}

	api, err := c.jimmAPIFunc()
	if err != nil {
		return err
	}
	defer api.Close()

	events, err := api.FindAuditEvents(&c.args)
	if err != nil {
		return err
	}

	err = c.out.Write(ctxt, events)
	if err != nil {
		return err
	}
	return nil
}

func (c *listAuditEventsCommand) newClient() (JIMMAPI, error) {
	currentController, err := c.store.CurrentController()
	if err != nil {
		return nil, fmt.Errorf("could not determine controller: %w", err)
	}

	apiCaller, err := c.NewAPIRootWithDialOpts(c.store, currentController, "", nil)
	if err != nil {
		return nil, err
	}

	return api.NewClient(apiCaller), nil
}

func formatTabular(writer io.Writer, value interface{}) error {
	e, ok := value.(apiparams.AuditEvents)
	if !ok {
		return fmt.Errorf("expected value of type %T, got %T", e, value)
	}

	table := uitable.New()
	table.MaxColWidth = 50
	table.Wrap = true

	table.AddRow("Time", "User", "Model", "ConversationId", "MessageId", "Method", "IsResponse", "Params", "Errors")
	for _, event := range e.Events {
		errorJSON, err := json.Marshal(event.Errors)
		if err != nil {
			return err
		}
		paramsJSON, err := json.Marshal(event.Params)
		if err != nil {
			return err
		}
		table.AddRow(event.Time, event.UserTag, event.Model, event.ConversationId, event.MessageId, event.FacadeMethod, event.IsResponse, string(paramsJSON), string(errorJSON))
	}
	fmt.Fprint(writer, table)
	return nil
}
