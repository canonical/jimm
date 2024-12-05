// Copyright 2024 Canonical.

package cmd

import (
	"bufio"
	"fmt"
	"strings"

	"github.com/juju/cmd/v3"
	jujucmdv3 "github.com/juju/cmd/v3"
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
	roleDoc = `
The role command enables role management for jimm
`

	addRoleDoc = `
The add command adds role to jimm.
`

	addRoleExample = `
    jimmctl auth role add myrole 
`

	renameRoleDoc = `
The rename command renames a role in jimm.
`
	renameRoleExample = `
    jimmctl auth role rename myrole newrolename
`

	removeRoleDoc = `
The remove command removes a role in jimm.
`

	removeRoleExample = `
    jimmctl auth role remove myrole
`

	listRolesDoc = `
The list command lists all roles in jimm.
`
	listRolesExample = `
    jimmctl auth role list
`
)

// NewRoleCommand returns a command for role management.
func NewRoleCommand() *jujucmdv3.SuperCommand {
	cmd := jujucmd.NewSuperCommand(jujucmdv3.SuperCommandParams{
		Name:    "role",
		Doc:     roleDoc,
		Purpose: "Role management.",
	})
	cmd.Register(newAddRoleCommand())
	cmd.Register(newRenameRoleCommand())
	cmd.Register(newRemoveRoleCommand())
	cmd.Register(newListRolesCommand())

	return cmd
}

// newAddRoleCommand returns a command to add a role.
func newAddRoleCommand() cmd.Command {
	cmd := &addRoleCommand{
		store: jujuclient.NewFileClientStore(),
	}

	return modelcmd.WrapBase(cmd)
}

// addRoleCommand adds a role.
type addRoleCommand struct {
	modelcmd.ControllerCommandBase
	out cmd.Output

	store    jujuclient.ClientStore
	dialOpts *jujuapi.DialOpts

	name string
}

// Info implements the cmd.Command interface.
func (c *addRoleCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "add",
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
		return errors.E("role name not specified")
	}
	c.name, args = args[0], args[1:]
	if len(args) > 0 {
		return errors.E("too many args")
	}
	return nil
}

// Run implements Command.Run.
func (c *addRoleCommand) Run(ctxt *cmd.Context) error {
	currentController, err := c.store.CurrentController()
	if err != nil {
		return errors.E(err, "could not determine controller")
	}

	apiCaller, err := c.NewAPIRootWithDialOpts(c.store, currentController, "", c.dialOpts)
	if err != nil {
		return err
	}

	client := api.NewClient(apiCaller)
	resp, err := client.AddRole(&apiparams.AddRoleRequest{
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

// newRenameRoleCommand returns a command to rename a role.
func newRenameRoleCommand() cmd.Command {
	cmd := &renameRoleCommand{
		store: jujuclient.NewFileClientStore(),
	}

	return modelcmd.WrapBase(cmd)
}

// renameRoleCommand renames a role.
type renameRoleCommand struct {
	modelcmd.ControllerCommandBase

	store    jujuclient.ClientStore
	dialOpts *jujuapi.DialOpts

	name    string
	newName string
}

// Info implements the cmd.Command interface.
func (c *renameRoleCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "rename",
		Args:     "<role name> <new role name>",
		Purpose:  "Rename a role.",
		Doc:      renameRoleDoc,
		Examples: renameRoleExample,
	})
}

// Init implements the cmd.Command interface.
func (c *renameRoleCommand) Init(args []string) error {
	if len(args) < 2 {
		return errors.E("role name not specified")
	}
	c.name, c.newName, args = args[0], args[1], args[2:]
	if len(args) > 0 {
		return errors.E("too many args")
	}
	return nil
}

// Run implements Command.Run.
func (c *renameRoleCommand) Run(ctxt *cmd.Context) error {
	currentController, err := c.store.CurrentController()
	if err != nil {
		return errors.E(err, "could not determine controller")
	}

	apiCaller, err := c.NewAPIRootWithDialOpts(c.store, currentController, "", c.dialOpts)
	if err != nil {
		return err
	}

	params := apiparams.RenameRoleRequest{
		Name:    c.name,
		NewName: c.newName,
	}

	client := api.NewClient(apiCaller)
	err = client.RenameRole(&params)
	if err != nil {
		return errors.E(err)
	}

	return nil
}

// newRemoveRoleCommand returns a command to Remove a role.
func newRemoveRoleCommand() cmd.Command {
	cmd := &removeRoleCommand{
		store: jujuclient.NewFileClientStore(),
	}

	return modelcmd.WrapBase(cmd)
}

// removeRoleCommand Removes a role.
type removeRoleCommand struct {
	modelcmd.ControllerCommandBase
	out cmd.Output

	store    jujuclient.ClientStore
	dialOpts *jujuapi.DialOpts

	name  string
	force bool
}

// Info implements the cmd.Command interface.
func (c *removeRoleCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "remove",
		Args:     "<role name>",
		Purpose:  "Remove a role.",
		Doc:      removeRoleDoc,
		Examples: removeRoleExample,
	})
}

// Init implements the cmd.Command interface.
func (c *removeRoleCommand) Init(args []string) error {
	if len(args) < 1 {
		return errors.E("role name not specified")
	}
	c.name, args = args[0], args[1:]
	if len(args) > 0 {
		return errors.E("too many args")
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
	currentController, err := c.store.CurrentController()
	if err != nil {
		return errors.E(err, "could not determine controller")
	}

	if !c.force {
		reader := bufio.NewReader(ctxt.Stdin)
		// Using Fprintf over c.out.write to avoid printing a new line.
		_, err := fmt.Fprintf(ctxt.Stdout, "This will also delete all associated relations.\nConfirm you would like to delete role %q (y/N): ", c.name)
		if err != nil {
			return err
		}
		text, err := reader.ReadString('\n')
		if err != nil {
			return errors.E(err, "Failed to read from input.")
		}
		text = strings.ReplaceAll(text, "\n", "")
		if !(text == "y" || text == "Y") {
			return nil
		}
	}

	apiCaller, err := c.NewAPIRootWithDialOpts(c.store, currentController, "", c.dialOpts)
	if err != nil {
		return err
	}

	params := apiparams.RemoveRoleRequest{
		Name: c.name,
	}

	client := api.NewClient(apiCaller)
	err = client.RemoveRole(&params)
	if err != nil {
		return errors.E(err)
	}

	return nil
}

// newListRolesCommand returns a command to list all roles.
func newListRolesCommand() cmd.Command {
	cmd := &listRolesCommand{
		store: jujuclient.NewFileClientStore(),
	}

	return modelcmd.WrapBase(cmd)
}

// listRolesCommand Lists all roles.
type listRolesCommand struct {
	modelcmd.ControllerCommandBase
	out cmd.Output

	store    jujuclient.ClientStore
	dialOpts *jujuapi.DialOpts

	limit  int
	offset int
}

// Info implements the cmd.Command interface.
func (c *listRolesCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "list",
		Purpose:  "List all roles.",
		Doc:      listRolesDoc,
		Examples: listRolesExample,
	})
}

// Init implements the cmd.Command interface.
func (c *listRolesCommand) Init(args []string) error {
	if len(args) > 1 {
		return errors.E("too many args")
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
	currentController, err := c.store.CurrentController()
	if err != nil {
		return errors.E(err, "could not determine controller")
	}

	apiCaller, err := c.NewAPIRootWithDialOpts(c.store, currentController, "", c.dialOpts)
	if err != nil {
		return err
	}

	client := api.NewClient(apiCaller)
	req := apiparams.ListRolesRequest{Limit: c.limit, Offset: c.offset}
	roles, err := client.ListRoles(&req)
	if err != nil {
		return errors.E(err)
	}

	err = c.out.Write(ctxt, roles)
	if err != nil {
		return errors.E(err)
	}

	return nil
}
