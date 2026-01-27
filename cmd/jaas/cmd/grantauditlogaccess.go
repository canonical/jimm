// Copyright 2025 Canonical.

package cmd

import (
	"errors"
	"fmt"

	"github.com/juju/cmd/v3"
	"github.com/juju/gnuflag"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/names/v5"

	"github.com/canonical/jimm/v3/pkg/api"
	apiparams "github.com/canonical/jimm/v3/pkg/api/params"
)

const (
	grantAuditLogAccessDoc = `
Grants a user access to read audit logs.
`

	grantAuditLogAccessExamples = `
    juju grant-audit-log <username>
`
)

// NewGrantAuditLogAccessCommand returns a command used to grant
// users access to audit logs.
func NewGrantAuditLogAccessCommand() cmd.Command {
	cmd := &grantAuditLogAccessCommand{
		store: jujuclient.NewFileClientStore(),
	}
	cmd.jimmAPIFunc = cmd.newClient

	return modelcmd.WrapBase(cmd)
}

// grantAuditLogAccessCommand displays full
// model status.
type grantAuditLogAccessCommand struct {
	modelcmd.ControllerCommandBase

	username string

	store       jujuclient.ClientStore
	jimmAPIFunc func() (JIMMAPI, error)
}

func (c *grantAuditLogAccessCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "grant-audit-log",
		Args:     "<username>",
		Purpose:  "Grants access to audit logs.",
		Doc:      grantAuditLogAccessDoc,
		Examples: grantAuditLogAccessExamples,
	})
}

// SetFlags implements Command.SetFlags.
func (c *grantAuditLogAccessCommand) SetFlags(f *gnuflag.FlagSet) {
	c.CommandBase.SetFlags(f)
}

// Init implements the cmd.Command interface.
func (c *grantAuditLogAccessCommand) Init(args []string) error {
	if len(args) == 0 {
		return errors.New("missing username")
	}

	c.username, args = args[0], args[1:]
	if len(args) > 0 {
		return errors.New("unknown arguments")
	}

	if !names.IsValidUser(c.username) {
		return fmt.Errorf("invalid username %q", c.username)
	}
	return nil
}

// Run implements Command.Run.
func (c *grantAuditLogAccessCommand) Run(ctxt *cmd.Context) error {
	if c.jimmAPIFunc == nil {
		c.jimmAPIFunc = c.newClient
	}

	api, err := c.jimmAPIFunc()
	if err != nil {
		return err
	}
	defer api.Close()

	err = api.GrantAuditLogAccess(&apiparams.AuditLogAccessRequest{
		UserTag: names.NewUserTag(c.username).String(),
	})
	if err != nil {
		return err
	}

	return nil
}

func (c *grantAuditLogAccessCommand) newClient() (JIMMAPI, error) {
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
