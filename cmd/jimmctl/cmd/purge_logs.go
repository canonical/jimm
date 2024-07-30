package cmd

import (
	"time"

	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/pkg/api"
	apiparams "github.com/canonical/jimm/v3/pkg/api/params"
	"github.com/juju/cmd/v3"
	"github.com/juju/gnuflag"
	jujuapi "github.com/juju/juju/api"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/jujuclient"
)

const purgeLogsDoc = `
	purge-audit-logs purges logs from the database before the given date.

	Examples:
		jimmctl purge-audit-logs 2021-02-03
		jimmctl purge-audit-logs 2021-02-03T00
		jimmctl purge-audit-logs 2021-02-03T15:04:05Z	
`

// NewPurgeLogsCommand returns a command to purge logs.
func NewPurgeLogsCommand() cmd.Command {
	cmd := &purgeLogsCommand{
		store: jujuclient.NewFileClientStore(),
	}
	return modelcmd.WrapBase(cmd)
}

// purgeLogsCommand purges logs.
type purgeLogsCommand struct {
	modelcmd.ControllerCommandBase
	store    jujuclient.ClientStore
	dialOpts *jujuapi.DialOpts
	out      cmd.Output

	date time.Time
}

// Info implements Command.Info. It returns the command information.
func (c *purgeLogsCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "purge-audit-logs",
		Args:    "<ISO8601 date>",
		Purpose: "purges audit logs from the database before the given date",
		Doc:     purgeLogsDoc,
	})
}

// Init implements Command.Init. It checks the number of arguments and validates
// the date.
func (c *purgeLogsCommand) Init(args []string) error {
	if len(args) != 1 {
		return errors.E("expected one argument (ISO8601 date)")
	}
	// validate date
	var err error
	c.date, err = parseDate(args[0])
	if err != nil {
		return errors.E("invalid date. Expected ISO8601 date")
	}
	return nil
}

// SetFlags implements Command.SetFlags.
func (c *purgeLogsCommand) SetFlags(f *gnuflag.FlagSet) {
	c.CommandBase.SetFlags(f)
	c.out.AddFlags(f, "yaml", map[string]cmd.Formatter{
		"yaml": cmd.FormatYaml,
		"json": cmd.FormatJson,
	})
}

// Run implements Command.Run. It purges logs from the database before the given
// date.
func (c *purgeLogsCommand) Run(ctx *cmd.Context) error {
	currentController, err := c.store.CurrentController()
	if err != nil {
		return errors.E(err, "could not determine controller")
	}

	apiCaller, err := c.NewAPIRootWithDialOpts(c.store, currentController, "", c.dialOpts)
	if err != nil {
		return err
	}

	client := api.NewClient(apiCaller)
	response, err := client.PurgeLogs(&apiparams.PurgeLogsRequest{
		Date: c.date,
	})
	if err != nil {
		return errors.E(err)
	}
	err = c.out.Write(ctx, response)
	if err != nil {
		return errors.E(err)
	}
	return nil
}

// parseDate validates the date string is in ISO8601 format. If it is, it
// sets the date field in the command.
func parseDate(date string) (time.Time, error) {
	// Define the possible ISO8601 date layouts
	layouts := []string{
		"2006-01-02T15:04:05-0700",
		"2006-01-02T15:04:05Z",
		"2006-01-02T15:04:05",
		"2006-01-02T15:04Z",
		"2006-01-02",
	}

	// Try to parse the date string using the defined layouts
	for _, layout := range layouts {
		date_time, err := time.Parse(layout, date)
		if err == nil {
			// If parsing was successful, the date is valid
			// You can use the parsed time t if needed
			return date_time, nil
		}
	}

	// If none of the layouts match, the date is not in the correct format
	return time.Time{}, errors.E("invalid date. Expected ISO8601 date")
}
