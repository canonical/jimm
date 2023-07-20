package cmd

import (
	"fmt"
	"time"

	"github.com/CanonicalLtd/jimm/api"
	apiparams "github.com/CanonicalLtd/jimm/api/params"
	"github.com/CanonicalLtd/jimm/internal/errors"
	"github.com/juju/cmd/v3"
	jujuapi "github.com/juju/juju/api"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/jujuclient"
)

const purgeLogsDoc = `
	purge-logs purges logs from the database before the given date.

	Example:
		jimmctl purge-logs 2021-01-01T00:00:00Z
`

// NewPurgeLogsCommand returns a command to purge logs.
func NewPurgeLogsCommand() cmd.Command {
	cmd := &purgeLogsCommand{}
	return modelcmd.WrapBase(cmd)
}

// purgeLogsCommand purges logs.
type purgeLogsCommand struct {
	modelcmd.ControllerCommandBase
	store    jujuclient.ClientStore
	dialOpts *jujuapi.DialOpts
	out      cmd.Output

	date string
}

func (c *purgeLogsCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "purge-logs",
		Args:    "<ISO8601 date>",
		Purpose: "purge logs from the database before the given date",
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
	err := c.validateDate(args[0])
	if err != nil {
		return errors.E("invalid date. Expected ISO8601 date")
	}

	return nil
}

// validateDate validates the date string is in ISO8601 format. If it is, it
// sets the date field in the command.
func (c *purgeLogsCommand) validateDate(date string) error {
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
		_, err := time.Parse(layout, date)
		if err == nil {
			// If parsing was successful, the date is valid
			// You can use the parsed time t if needed
			c.date = date
			return nil
		}
	}

	// If none of the layouts match, the date is not in the correct format
	return fmt.Errorf("invalid date. Expected ISO8601 date")

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
	fmt.Fprintf(ctx.Stdout, "Deleted %d logs\n", response.DeletedCount)
	return nil
}
