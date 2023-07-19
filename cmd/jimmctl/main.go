// Copyright 2021 Canonical Ltd.

package main

import (
	"fmt"
	"os"

	jujucmd "github.com/juju/cmd/v3"

	"github.com/canonical/jimm/cmd/jimmctl/cmd"
)

var jimmctlDoc = `
jimmctl enables users to manage JIMM.
`

func NewSuperCommand() *jujucmd.SuperCommand {
	jimmcmd := jujucmd.NewSuperCommand(jujucmd.SuperCommandParams{
		Name: "jimmctl",
		Doc:  jimmctlDoc,
	})
	jimmcmd.Register(cmd.NewAddControllerCommand())
	jimmcmd.Register(cmd.NewControllerInfoCommand())
	jimmcmd.Register(cmd.NewGrantAuditLogAccessCommand())
	jimmcmd.Register(cmd.NewImportCloudCredentialsCommand())
	jimmcmd.Register(cmd.NewImportModelCommand())
	jimmcmd.Register(cmd.NewListAuditEventsCommand())
	jimmcmd.Register(cmd.NewListControllersCommand())
	jimmcmd.Register(cmd.NewModelStatusCommand())
	jimmcmd.Register(cmd.NewRemoveControllerCommand())
	jimmcmd.Register(cmd.NewRevokeAuditLogAccessCommand())
	jimmcmd.Register(cmd.NewSetControllerDeprecatedCommand())
	jimmcmd.Register(cmd.NewUpdateMigratedModelCommand())
	jimmcmd.Register(cmd.NewAddCloudToControllerCommand())
	jimmcmd.Register(cmd.NewRemoveCloudFromControllerCommand())
	jimmcmd.Register(cmd.NewAuthCommand())
	jimmcmd.Register(cmd.NewCrossModelQueryCommand())
	return jimmcmd
}

func main() {
	ctx, err := jujucmd.DefaultContext()
	if err != nil {
		fmt.Printf("failed to get command context: %v\n", err)
		os.Exit(2)
	}
	superCmd := NewSuperCommand()
	args := os.Args

	os.Exit(jujucmd.Main(superCmd, ctx, args[1:]))
}
