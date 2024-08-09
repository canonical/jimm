// Copyright 2024 Canonical.

package cmd

import (
	"github.com/juju/cmd/v3"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	jujuapi "github.com/juju/juju/api"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/jujuclient"

	"github.com/canonical/jimm/v3/pkg/api"
	apiparams "github.com/canonical/jimm/v3/pkg/api/params"
)

var (
	// stdinMarkers contains file names that are taken to be stdin.
	crossModelQueryDoc = `
query-models command queries all models available to the current user
performing the query against each model status individually, returning
the collated query responses for each model.

The query will run against the exact output of "juju status --format json",
as such you can format your query against an output like this.

The queries will expect a JQ query string.

Example:
	jimmctl query-models '.applications | with_entries(select(.key=="nginx-ingress-integrator"))'
`
)

// crossModelQueryCommand queries all models available to the current
// user.
type crossModelQueryCommand struct {
	modelcmd.ControllerCommandBase
	out cmd.Output
	// query holds the query the user wishes to run against their models.
	query string
	// queryType holds the type of query the user wishes to use.
	queryType string

	store    jujuclient.ClientStore
	dialOpts *jujuapi.DialOpts
	file     cmd.FileVar
}

// NewCrossModelQueryCommand returns a command to query all of the models
// available to the current user.
func NewCrossModelQueryCommand() cmd.Command {
	cmd := &crossModelQueryCommand{
		store: jujuclient.NewFileClientStore(),
	}

	return modelcmd.WrapBase(cmd)
}

// Init implements modelcmd.Command.
func (c *crossModelQueryCommand) Init(args []string) error {
	if len(args) < 1 {
		return errors.New("no query specified")
	}
	c.query = args[0]
	c.queryType = "jq"
	return nil
}

// SetFlags implements modelcmd.Command.
func (c *crossModelQueryCommand) SetFlags(f *gnuflag.FlagSet) {
	c.CommandBase.SetFlags(f)
	c.out.AddFlags(f, "json", map[string]cmd.Formatter{
		"yaml": cmd.FormatYaml,
		"json": cmd.FormatJson,
	})
	c.file.StdinMarkers = stdinMarkers
}

// Info implements modelcmd.Command.
func (c *crossModelQueryCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "query-models",
		Purpose: "Query model statuses",
		Doc:     crossModelQueryDoc,
	})
}

// Run implements modelcmd.Command.
func (c *crossModelQueryCommand) Run(ctxt *cmd.Context) error {
	currentController, err := c.store.CurrentController()
	if err != nil {
		return errors.Annotate(err, "could not determine controller")
	}

	apiCaller, err := c.NewAPIRootWithDialOpts(c.store, currentController, "", c.dialOpts)
	if err != nil {
		return err
	}

	req := apiparams.CrossModelQueryRequest{
		Type:  c.queryType,
		Query: c.query,
	}

	client := api.NewClient(apiCaller)
	resp, err := client.CrossModelQuery(&req)
	if err != nil {
		return errors.Mask(err)
	}

	err = c.out.Write(ctxt, resp)
	if err != nil {
		return errors.Mask(err)
	}
	return nil
}
