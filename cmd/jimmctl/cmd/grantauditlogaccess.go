// Copyright 2021 Canonical Ltd.

package cmd

import (
	"github.com/juju/cmd/v3"
	"github.com/juju/gnuflag"
	jujuapi "github.com/juju/juju/api"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/names/v5"

	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/pkg/api"
	apiparams "github.com/canonical/jimm/v3/pkg/api/params"
)

var grantAuditLogAccessDoc = `
	grant-audit-log-access grants user access to audit logs.

	Example:
		jimmctl grant-audit-log-access <username> 
`

// NewGrantAuditLogAccessCommand returns a command used to grant
// users access to audit logs.
func NewGrantAuditLogAccessCommand() cmd.Command {
	cmd := &grantAuditLogAccessCommand{
		store: jujuclient.NewFileClientStore(),
	}

	return modelcmd.WrapBase(cmd)
}

// grantAuditLogAccessCommand displays full
// model status.
type grantAuditLogAccessCommand struct {
	modelcmd.ControllerCommandBase

	store    jujuclient.ClientStore
	dialOpts *jujuapi.DialOpts
	username string
}

func (c *grantAuditLogAccessCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "grant-audit-log-access",
		Purpose: "Grants access to audit logs.",
		Doc:     grantAuditLogAccessDoc,
	})
}

// SetFlags implements Command.SetFlags.
func (c *grantAuditLogAccessCommand) SetFlags(f *gnuflag.FlagSet) {
	c.CommandBase.SetFlags(f)
}

// Init implements the cmd.Command interface.
func (c *grantAuditLogAccessCommand) Init(args []string) error {
	if len(args) == 0 {
		return errors.E("missing username")
	}
	c.username, args = args[0], args[1:]
	if len(args) > 0 {
		return errors.E("unknown arguments")
	}
	return nil
}

// Run implements Command.Run.
func (c *grantAuditLogAccessCommand) Run(ctxt *cmd.Context) error {
	currentController, err := c.store.CurrentController()
	if err != nil {
		return errors.E(err, "could not determine controller")
	}

	userTag := names.NewUserTag(c.username)
	apiCaller, err := c.NewAPIRootWithDialOpts(c.store, currentController, "", c.dialOpts)
	if err != nil {
		return err
	}

	client := api.NewClient(apiCaller)
	err = client.GrantAuditLogAccess(&apiparams.AuditLogAccessRequest{
		UserTag: userTag.String(),
	})
	if err != nil {
		return errors.E(err)
	}

	return nil
}
