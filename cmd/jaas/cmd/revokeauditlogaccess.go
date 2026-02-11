// Copyright 2026 Canonical.

package cmd

import (
	"fmt"

	"github.com/juju/cmd/v3"
	"github.com/juju/gnuflag"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/names/v5"

	apiparams "github.com/canonical/jimm/v3/pkg/api/params"
)

const (
	revokeAuditLogAccessDoc = `
Revokes user access to audit logs.
`
	revokeAuditLogAccessExample = `
    juju revoke-audit-log user@canonical.com
`
)

// NewrevokeAuditLogAccess returns a command used to revoke
// users access to audit logs.
func NewRevokeAuditLogAccessCommand() cmd.Command {
	cmd := &revokeAuditLogAccessCommand{}
	cmd.SetClientStore(jujuclient.NewFileClientStore())

	return modelcmd.WrapBase(cmd)
}

// revokeAuditLogAccess displays full
// model status.
type revokeAuditLogAccessCommand struct {
	jaasCommandBase

	username string
}

func (c *revokeAuditLogAccessCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "revoke-audit-log",
		Args:     "<user>",
		Purpose:  "revokes access to audit logs.",
		Doc:      revokeAuditLogAccessDoc,
		Examples: revokeAuditLogAccessExample,
	})
}

// SetFlags implements Command.SetFlags.
func (c *revokeAuditLogAccessCommand) SetFlags(f *gnuflag.FlagSet) {
	c.CommandBase.SetFlags(f)
}

// Init implements the cmd.Command interface.
func (c *revokeAuditLogAccessCommand) Init(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("missing username")
	}
	c.username, args = args[0], args[1:]
	if len(args) > 0 {
		return fmt.Errorf("unknown arguments")
	}
	return nil
}

// Run implements Command.Run.
func (c *revokeAuditLogAccessCommand) Run(ctxt *cmd.Context) error {
	client, err := c.getJIMMAPI()
	if err != nil {
		return err
	}
	defer client.Close()

	err = client.RevokeAuditLogAccess(&apiparams.AuditLogAccessRequest{
		UserTag: names.NewUserTag(c.username).String(),
	})
	if err != nil {
		return err
	}

	return nil
}
