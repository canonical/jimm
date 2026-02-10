// Copyright 2025 Canonical.

package cmd

import (
	"bufio"
	"fmt"
	"strings"

	"github.com/juju/cmd/v3"
	"github.com/juju/gnuflag"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/jujuclient"

	"github.com/canonical/jimm/v3/pkg/api"
	apiparams "github.com/canonical/jimm/v3/pkg/api/params"
)

const (
	addRoleDoc = `
Adds a role.
`

	addRoleExample = `
    juju add-role myrole
`

	renameRoleDoc = `
Renames a role.
`
	renameRoleExample = `
    juju rename-role myrole newrolename
`

	removeRoleDoc = `
Removes a role.
`

	removeRoleExample = `
    juju remove-role remove myrole
`

	listRolesDoc = `
Lists all roles.
`
	listRolesExample = `
    juju list-roles list
`
)

// NewAddRoleCommand returns a command to add a role.
func NewAddRoleCommand() cmd.Command {
	cmd := &addRoleCommand{}
	cmd.SetClientStore(jujuclient.NewFileClientStore())

	return modelcmd.WrapBase(cmd)
}

// addRoleCommand adds a role.
type addRoleCommand struct {
	modelcmd.ControllerCommandBase
	out cmd.Output

	jimmAPIFunc func() (JIMMAPI, error)

	name string
}

// Info implements the cmd.Command interface.
func (c *addRoleCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "add-role",
		Args:     "<role name>",
		Purpose:  "Add role to jimm.",
		Doc:      addRoleDoc,
		Examples: addRoleExample,
	})
}

// SetFlags implements Command.SetFlags.
func (c *addRoleCommand) SetFlags(f *gnuflag.FlagSet) {
	c.CommandBase.SetFlags(f)
	c.out.AddFlags(f, "yaml", map[string]cmd.Formatter{
		"yaml": cmd.FormatYaml,
		"json": cmd.FormatJson,
	})
}

// Init implements the cmd.Command interface.
func (c *addRoleCommand) Init(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("role name not specified")
	}
	c.name, args = args[0], args[1:]
	if len(args) > 0 {
		return fmt.Errorf("too many args")
	}
	return nil
}

// Run implements Command.Run.
func (c *addRoleCommand) Run(ctxt *cmd.Context) error {
	if c.jimmAPIFunc == nil {
		c.jimmAPIFunc = c.newClient
	}
	client, err := c.jimmAPIFunc()
	if err != nil {
		return err
	}
	defer client.Close()

	resp, err := client.AddRole(&apiparams.AddRoleRequest{
		Name: c.name,
	})
	if err != nil {
		return err
	}

	err = c.out.Write(ctxt, resp)
	if err != nil {
		return err
	}
	return nil
}

func (c *addRoleCommand) newClient() (JIMMAPI, error) {
	currentController, err := c.ClientStore().CurrentController()
	if err != nil {
		return nil, fmt.Errorf("could not determine controller: %w", err)
	}

	apiCaller, err := c.NewAPIRootWithDialOpts(c.ClientStore(), currentController, "", nil)
	if err != nil {
		return nil, err
	}

	return api.NewClient(apiCaller), nil
}

// NewRenameRoleCommand returns a command to rename a role.
func NewRenameRoleCommand() cmd.Command {
	cmd := &renameRoleCommand{}
	cmd.SetClientStore(jujuclient.NewFileClientStore())

	return modelcmd.WrapBase(cmd)
}

// renameRoleCommand renames a role.
type renameRoleCommand struct {
	modelcmd.ControllerCommandBase

	jimmAPIFunc func() (JIMMAPI, error)

	name    string
	newName string
}

// Info implements the cmd.Command interface.
func (c *renameRoleCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "rename-role",
		Args:     "<role name> <new role name>",
		Purpose:  "Rename a role.",
		Doc:      renameRoleDoc,
		Examples: renameRoleExample,
	})
}

// Init implements the cmd.Command interface.
func (c *renameRoleCommand) Init(args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("role name not specified")
	}
	c.name, c.newName, args = args[0], args[1], args[2:]
	if len(args) > 0 {
		return fmt.Errorf("too many args")
	}
	return nil
}

// Run implements Command.Run.
func (c *renameRoleCommand) Run(ctxt *cmd.Context) error {
	if c.jimmAPIFunc == nil {
		c.jimmAPIFunc = c.newClient
	}
	client, err := c.jimmAPIFunc()
	if err != nil {
		return err
	}
	defer client.Close()

	params := apiparams.RenameRoleRequest{
		Name:    c.name,
		NewName: c.newName,
	}

	err = client.RenameRole(&params)
	if err != nil {
		return err
	}

	return nil
}

func (c *renameRoleCommand) newClient() (JIMMAPI, error) {
	currentController, err := c.ClientStore().CurrentController()
	if err != nil {
		return nil, fmt.Errorf("could not determine controller: %w", err)
	}

	apiCaller, err := c.NewAPIRootWithDialOpts(c.ClientStore(), currentController, "", nil)
	if err != nil {
		return nil, err
	}

	return api.NewClient(apiCaller), nil
}

// NewRemoveRoleCommand returns a command to Remove a role.
func NewRemoveRoleCommand() cmd.Command {
	cmd := &removeRoleCommand{}
	cmd.SetClientStore(jujuclient.NewFileClientStore())

	return modelcmd.WrapBase(cmd)
}

// removeRoleCommand Removes a role.
type removeRoleCommand struct {
	modelcmd.ControllerCommandBase
	out cmd.Output

	jimmAPIFunc func() (JIMMAPI, error)

	name  string
	force bool
}

// Info implements the cmd.Command interface.
func (c *removeRoleCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "remove-role",
		Args:     "<role name>",
		Purpose:  "Remove a role.",
		Doc:      removeRoleDoc,
		Examples: removeRoleExample,
	})
}

// Init implements the cmd.Command interface.
func (c *removeRoleCommand) Init(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("role name not specified")
	}
	c.name, args = args[0], args[1:]
	if len(args) > 0 {
		return fmt.Errorf("too many args")
	}
	return nil
}

// SetFlags implements Command.SetFlags.
func (c *removeRoleCommand) SetFlags(f *gnuflag.FlagSet) {
	c.CommandBase.SetFlags(f)
	c.out.AddFlags(f, "smart", map[string]cmd.Formatter{
		"smart": cmd.FormatSmart,
	})
	f.BoolVar(&c.force, "y", false, "delete role without prompt")
}

// Run implements Command.Run.
func (c *removeRoleCommand) Run(ctxt *cmd.Context) error {
	if !c.force {
		reader := bufio.NewReader(ctxt.Stdin)
		// Using Fprintf over c.out.write to avoid printing a new line.
		_, err := fmt.Fprintf(ctxt.Stdout, "This will also delete all associated relations.\nConfirm you would like to delete role %q (y/N): ", c.name)
		if err != nil {
			return err
		}
		text, err := reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("failed to read from input: %w", err)
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
		return err
	}
	defer client.Close()

	params := apiparams.RemoveRoleRequest{
		Name: c.name,
	}

	err = client.RemoveRole(&params)
	if err != nil {
		return err
	}

	return nil
}

func (c *removeRoleCommand) newClient() (JIMMAPI, error) {
	currentController, err := c.ClientStore().CurrentController()
	if err != nil {
		return nil, fmt.Errorf("could not determine controller: %w", err)
	}

	apiCaller, err := c.NewAPIRootWithDialOpts(c.ClientStore(), currentController, "", nil)
	if err != nil {
		return nil, err
	}

	return api.NewClient(apiCaller), nil
}

// NewListRolesCommand returns a command to list all roles.
func NewListRolesCommand() cmd.Command {
	cmd := &listRolesCommand{}
	cmd.SetClientStore(jujuclient.NewFileClientStore())

	return modelcmd.WrapBase(cmd)
}

// listRolesCommand Lists all roles.
type listRolesCommand struct {
	modelcmd.ControllerCommandBase
	out cmd.Output

	jimmAPIFunc func() (JIMMAPI, error)

	limit  int
	offset int
}

// Info implements the cmd.Command interface.
func (c *listRolesCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "list-roles",
		Purpose:  "List all roles.",
		Doc:      listRolesDoc,
		Examples: listRolesExample,
		Aliases:  []string{"roles"},
	})
}

// Init implements the cmd.Command interface.
func (c *listRolesCommand) Init(args []string) error {
	if len(args) > 1 {
		return fmt.Errorf("too many args")
	}
	return nil
}

// SetFlags implements Command.SetFlags.
func (c *listRolesCommand) SetFlags(f *gnuflag.FlagSet) {
	c.CommandBase.SetFlags(f)
	c.out.AddFlags(f, "yaml", map[string]cmd.Formatter{
		"yaml": cmd.FormatYaml,
		"json": cmd.FormatJson,
	})
	f.IntVar(&c.limit, "limit", 0, "The maximum number of roles to return")
	f.IntVar(&c.offset, "offset", 0, "The offset to use when requesting roles")
}

// Run implements Command.Run.
func (c *listRolesCommand) Run(ctxt *cmd.Context) error {
	if c.jimmAPIFunc == nil {
		c.jimmAPIFunc = c.newClient
	}
	client, err := c.jimmAPIFunc()
	if err != nil {
		return err
	}
	defer client.Close()

	req := apiparams.ListRolesRequest{Limit: c.limit, Offset: c.offset}
	roles, err := client.ListRoles(&req)
	if err != nil {
		return err
	}

	err = c.out.Write(ctxt, roles)
	if err != nil {
		return err
	}

	return nil
}

func (c *listRolesCommand) newClient() (JIMMAPI, error) {
	currentController, err := c.ClientStore().CurrentController()
	if err != nil {
		return nil, fmt.Errorf("could not determine controller: %w", err)
	}

	apiCaller, err := c.NewAPIRootWithDialOpts(c.ClientStore(), currentController, "", nil)
	if err != nil {
		return nil, err
	}

	return api.NewClient(apiCaller), nil
}
