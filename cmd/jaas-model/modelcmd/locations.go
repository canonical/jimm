// Copyright 2016 Canonical Ltd.

package modelcmd

import (
	"fmt"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/utils/keyvalues"
	"gopkg.in/errgo.v1"

	"github.com/CanonicalLtd/jem/params"
)

type locationsCommand struct {
	commandBase
	attributes map[string]string
}

func newLocationsCommand() cmd.Command {
	return modelcmd.WrapBase(&locationsCommand{})
}

// TODO(rog) add --format flag so that we can
// obtain tool-readable output.

var locationsDoc = `
The locations command lists locations of available
controllers. If any key=value arguments are provided,
the controllers will be restricted to those with matching
locations.

For example,

	jaas model locations cloud=aws

will print all locations that have the cloud location
set to "aws".
`

func (c *locationsCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "locations",
		Args:    "[<key>=<value>]...",
		Purpose: "show available controller locations",
		Doc:     locationsDoc,
	}
}

func (c *locationsCommand) Init(args []string) error {
	attrs, err := keyvalues.Parse(args, false)
	if err != nil {
		return errgo.Mask(err)
	}
	c.attributes = attrs
	return nil
}

func (c *locationsCommand) Run(ctxt *cmd.Context) error {
	client, err := c.newClient(ctxt)
	if err != nil {
		return errgo.Mask(err)
	}
	resp, err := client.GetAllControllerLocations(&params.GetAllControllerLocations{
		Location: c.attributes,
	})
	if err != nil {
		return errgo.Mask(err)
	}
	if len(resp.Locations) == 0 {
		return nil
	}
	columnMap := make(map[string]bool)
	for _, loc := range resp.Locations {
		for k := range loc {
			columnMap[k] = true
		}
	}
	columns := make([]string, 0, len(columnMap))
	for k := range columnMap {
		columns = append(columns, k)
	}
	sort.Strings(columns) // Could be more sophisticated here.
	tw := tabwriter.NewWriter(ctxt.Stdout, 0, 1, 1, ' ', 0)
	fmt.Fprintln(tw, strings.ToUpper(strings.Join(columns, "\t")))
	sort.Sort(&locationsByColumn{
		locations: resp.Locations,
		columns:   columns,
	})
	for _, loc := range resp.Locations {
		for i, col := range columns {
			if i > 0 {
				fmt.Fprint(tw, "\t")
			}
			fmt.Fprint(tw, loc[col])
		}
		fmt.Fprintln(tw)
	}
	tw.Flush()
	return nil
}

type locationsByColumn struct {
	locations []map[string]string
	columns   []string
}

func (c *locationsByColumn) Len() int {
	return len(c.locations)
}

func (c *locationsByColumn) Swap(i, j int) {
	c.locations[i], c.locations[j] = c.locations[j], c.locations[i]
}

func (c *locationsByColumn) Less(i, j int) bool {
	l1 := c.locations[i]
	l2 := c.locations[j]
	for _, col := range c.columns {
		v1, v2 := l1[col], l2[col]
		if v1 != v2 {
			return v1 < v2
		}
	}
	return false
}
