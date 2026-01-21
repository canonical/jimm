// Copyright 2025 Canonical.

package cmd

import (
	"bufio"
	"fmt"
	"strings"

	"github.com/juju/cmd/v3"
	"github.com/juju/gnuflag"
	jujuapi "github.com/juju/juju/api"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/jujuclient"

	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/pkg/api"
	apiparams "github.com/canonical/jimm/v3/pkg/api/params"
)

const (
	addGroupDoc = `
Adds a group.
`
	addGroupExample = `
    juju add-group
`
	renameGroupDoc = `
Renames a group.
`
	renameGroupExample = `
    juju rename-group mygroup newgroup
`
	removeGroupDoc = `
Removes a group.
`

	removeGroupExample = `
    juju remove-group mygroup
`

	listGroupsDoc = `
Lists all groups.
`
	listGroupsExample = `
    juju list-groups
`
)

// NewAddGroupCommand returns a command to add a group.
func NewAddGroupCommand() cmd.Command {
	cmd := &addGroupCommand{
		store: jujuclient.NewFileClientStore(),
	}
	cmd.jimmAPIFunc = cmd.newClient

	return modelcmd.WrapBase(cmd)
}

// addGroupCommand adds a group.
type addGroupCommand struct {
	modelcmd.ControllerCommandBase
	out cmd.Output

	store       jujuclient.ClientStore
	dialOpts    *jujuapi.DialOpts
	jimmAPIFunc func() (JIMMAPI, error)

	name string
}

// Info implements the cmd.Command interface.
func (c *addGroupCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "add-group",
		Args:     "<name>",
		Purpose:  "Add group to jimm.",
		Doc:      addGroupDoc,
		Examples: addGroupExample,
	})
}

// SetFlags implements Command.SetFlags.
func (c *addGroupCommand) SetFlags(f *gnuflag.FlagSet) {
	c.CommandBase.SetFlags(f)
	c.out.AddFlags(f, "yaml", map[string]cmd.Formatter{
		"yaml": cmd.FormatYaml,
		"json": cmd.FormatJson,
	})
}

// Init implements the cmd.Command interface.
func (c *addGroupCommand) Init(args []string) error {
	if len(args) < 1 {
		return errors.E("group name not specified")
	}
	c.name, args = args[0], args[1:]
	if len(args) > 0 {
		return errors.E("too many args")
	}
	return nil
}

// Run implements Command.Run.
func (c *addGroupCommand) Run(ctxt *cmd.Context) error {
	if c.jimmAPIFunc == nil {
		c.jimmAPIFunc = c.newClient
	}
	client, err := c.jimmAPIFunc()
	if err != nil {
		return errors.E(fmt.Errorf("could not create JIMM client: %v", err))
	}
	defer client.Close()

	resp, err := client.AddGroup(&apiparams.AddGroupRequest{
		Name: c.name,
	})
	if err != nil {
		return errors.E(err)
	}

	err = c.out.Write(ctxt, resp)
	if err != nil {
		return errors.E(err)
	}
	return nil
}

func (c *addGroupCommand) newClient() (JIMMAPI, error) {
	currentController, err := c.store.CurrentController()
	if err != nil {
		return nil, errors.E(fmt.Errorf("could not determine controller: %v", err))
	}

	apiCaller, err := c.NewAPIRootWithDialOpts(c.store, currentController, "", c.dialOpts)
	if err != nil {
		return nil, err
	}

	return api.NewClient(apiCaller), nil
}

// NewRenameGroupCommand returns a command to rename a group.
func NewRenameGroupCommand() cmd.Command {
	cmd := &renameGroupCommand{
		store: jujuclient.NewFileClientStore(),
	}
	cmd.jimmAPIFunc = cmd.newClient

	return modelcmd.WrapBase(cmd)
}

// renameGroupCommand renames a group.
type renameGroupCommand struct {
	modelcmd.ControllerCommandBase

	store       jujuclient.ClientStore
	dialOpts    *jujuapi.DialOpts
	jimmAPIFunc func() (JIMMAPI, error)

	name    string
	newName string
}

// Info implements the cmd.Command interface.
func (c *renameGroupCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "rename-group",
		Args:     "<name> <new name>",
		Purpose:  "Rename a group.",
		Doc:      renameGroupDoc,
		Examples: renameGroupExample,
	})
}

// Init implements the cmd.Command interface.
func (c *renameGroupCommand) Init(args []string) error {
	if len(args) < 2 {
		return errors.E("group name not specified")
	}
	c.name, c.newName, args = args[0], args[1], args[2:]
	if len(args) > 0 {
		return errors.E("too many args")
	}
	return nil
}

// Run implements Command.Run.
func (c *renameGroupCommand) Run(ctxt *cmd.Context) error {
	if c.jimmAPIFunc == nil {
		c.jimmAPIFunc = c.newClient
	}
	client, err := c.jimmAPIFunc()
	if err != nil {
		return errors.E("could not create JIMM client: %v", err)
	}
	defer client.Close()

	params := apiparams.RenameGroupRequest{
		Name:    c.name,
		NewName: c.newName,
	}

	err = client.RenameGroup(&params)
	if err != nil {
		return errors.E(err)
	}

	return nil
}

func (c *renameGroupCommand) newClient() (JIMMAPI, error) {
	currentController, err := c.store.CurrentController()
	if err != nil {
		return nil, errors.E(fmt.Errorf("could not determine controller: %v", err))
	}

	apiCaller, err := c.NewAPIRootWithDialOpts(c.store, currentController, "", c.dialOpts)
	if err != nil {
		return nil, err
	}

	return api.NewClient(apiCaller), nil
}

// NewRemoveGroupCommand returns a command to Remove a group.
func NewRemoveGroupCommand() cmd.Command {
	cmd := &removeGroupCommand{
		store: jujuclient.NewFileClientStore(),
	}
	cmd.jimmAPIFunc = cmd.newClient

	return modelcmd.WrapBase(cmd)
}

// removeGroupCommand Removes a group.
type removeGroupCommand struct {
	modelcmd.ControllerCommandBase
	out cmd.Output

	store       jujuclient.ClientStore
	dialOpts    *jujuapi.DialOpts
	jimmAPIFunc func() (JIMMAPI, error)

	name  string
	force bool
}

// Info implements the cmd.Command interface.
func (c *removeGroupCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "remove-group",
		Args:     "<name>",
		Purpose:  "Remove a group.",
		Doc:      removeGroupDoc,
		Examples: removeGroupExample,
	})
}

// Init implements the cmd.Command interface.
func (c *removeGroupCommand) Init(args []string) error {
	if len(args) < 1 {
		return errors.E("group name not specified")
	}
	c.name, args = args[0], args[1:]
	if len(args) > 0 {
		return errors.E("too many args")
	}
	return nil
}

// SetFlags implements Command.SetFlags.
func (c *removeGroupCommand) SetFlags(f *gnuflag.FlagSet) {
	c.CommandBase.SetFlags(f)
	c.out.AddFlags(f, "smart", map[string]cmd.Formatter{
		"smart": cmd.FormatSmart,
	})
	f.BoolVar(&c.force, "force", false, "delete group without prompt")
}

// Run implements Command.Run.
func (c *removeGroupCommand) Run(ctxt *cmd.Context) error {
	if !c.force {
		reader := bufio.NewReader(ctxt.Stdin)
		// Using Fprintf over c.out.write to avoid printing a new line.
		_, err := fmt.Fprintf(ctxt.Stdout, "This will also delele all associated relations.\nConfirm you would like to delete group %q (y/N): ", c.name)
		if err != nil {
			return err
		}
		text, err := reader.ReadString('\n')
		if err != nil {
			return errors.E(err, "Failed to read from input.")
		}
		text = strings.ReplaceAll(text, "\n", "")
		if text != "y" && text != "Y" {
			return nil
		}
	}
	if c.jimmAPIFunc == nil {
		c.jimmAPIFunc = c.newClient
	}

	client, err := c.jimmAPIFunc()
	if err != nil {
		return errors.E(fmt.Errorf("could not create JIMM client: %v", err))
	}
	defer client.Close()

	params := apiparams.RemoveGroupRequest{
		Name: c.name,
	}

	err = client.RemoveGroup(&params)
	if err != nil {
		return errors.E(err)
	}

	return nil
}

func (c *removeGroupCommand) newClient() (JIMMAPI, error) {
	currentController, err := c.store.CurrentController()
	if err != nil {
		return nil, errors.E(fmt.Errorf("could not determine controller: %v", err))
	}

	apiCaller, err := c.NewAPIRootWithDialOpts(c.store, currentController, "", c.dialOpts)
	if err != nil {
		return nil, err
	}

	return api.NewClient(apiCaller), nil
}

// NewListGroupsCommand returns a command to list all groups.
func NewListGroupsCommand() cmd.Command {
	cmd := &listGroupsCommand{
		store: jujuclient.NewFileClientStore(),
	}
	cmd.jimmAPIFunc = cmd.newClient

	return modelcmd.WrapBase(cmd)
}

// listGroupsCommand Lists all groups.
type listGroupsCommand struct {
	modelcmd.ControllerCommandBase
	out cmd.Output

	store       jujuclient.ClientStore
	dialOpts    *jujuapi.DialOpts
	jimmAPIFunc func() (JIMMAPI, error)

	limit  int
	offset int
}

// Info implements the cmd.Command interface.
func (c *listGroupsCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "list-groups",
		Purpose:  "List all groups.",
		Doc:      listGroupsDoc,
		Examples: listGroupsExample,
		Aliases:  []string{"groups"},
	})
}

// Init implements the cmd.Command interface.
func (c *listGroupsCommand) Init(args []string) error {
	if len(args) > 0 {
		return errors.E("too many args")
	}
	return nil
}

// SetFlags implements Command.SetFlags.
func (c *listGroupsCommand) SetFlags(f *gnuflag.FlagSet) {
	c.CommandBase.SetFlags(f)
	c.out.AddFlags(f, "yaml", map[string]cmd.Formatter{
		"yaml": cmd.FormatYaml,
		"json": cmd.FormatJson,
	})
	f.IntVar(&c.limit, "limit", 0, "The maximum number of groups to return")
	f.IntVar(&c.offset, "offset", 0, "The offset to use when requesting groups")
}

// Run implements Command.Run.
func (c *listGroupsCommand) Run(ctxt *cmd.Context) error {
	if c.jimmAPIFunc == nil {
		c.jimmAPIFunc = c.newClient
	}
	client, err := c.jimmAPIFunc()
	if err != nil {
		return errors.E("could not create JIMM client: %v", err)
	}
	defer client.Close()

	req := apiparams.ListGroupsRequest{Limit: c.limit, Offset: c.offset}
	groups, err := client.ListGroups(&req)
	if err != nil {
		return errors.E(err)
	}

	err = c.out.Write(ctxt, groups)
	if err != nil {
		return errors.E(err)
	}

	return nil
}

func (c *listGroupsCommand) newClient() (JIMMAPI, error) {
	currentController, err := c.store.CurrentController()
	if err != nil {
		return nil, errors.E(fmt.Errorf("could not determine controller: %v", err))
	}

	apiCaller, err := c.NewAPIRootWithDialOpts(c.store, currentController, "", c.dialOpts)
	if err != nil {
		return nil, err
	}

	return api.NewClient(apiCaller), nil
}
