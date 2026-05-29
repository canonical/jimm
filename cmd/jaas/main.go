// Copyright 2026 Canonical.

package main

import (
	"fmt"
	"os"
	"strings"

	jujucmd "github.com/juju/cmd/v3"

	"github.com/canonical/jimm/v3/cmd/jaas/cmd"
)

var jaasDoc = `
jaas enables users to use JAAS commands from within the Juju CLI.

JAAS enables enterprise functionality on top of Juju to provide
functionality like OIDC login, control over many controllers,
group management, and fine-grained authorisation.
`

func NewSuperCommand() *jujucmd.SuperCommand {
	jaasCmd := jujucmd.NewSuperCommand(jujucmd.SuperCommandParams{
		Name: "jaas",
		Doc:  jaasDoc,
	})

	// Register commands here:
	jaasCmd.Register(cmd.NewAddCloudToControllerCommand())
	jaasCmd.Register(cmd.NewAddControllerProfileCommand())
	jaasCmd.Register(cmd.NewAddGroupCommand())
	jaasCmd.Register(cmd.NewAddModelCommand())
	jaasCmd.Register(cmd.NewAddPermissionCommand())
	jaasCmd.Register(cmd.NewAddRoleCommand())
	jaasCmd.Register(cmd.NewCheckPermissionCommand())
	jaasCmd.Register(cmd.NewCrossModelQueryCommand())
	jaasCmd.Register(cmd.NewGrantAuditLogAccessCommand())
	jaasCmd.Register(cmd.NewImportModelCommand())
	jaasCmd.Register(cmd.NewListAuditEventsCommand())
	jaasCmd.Register(cmd.NewListControllerProfilesCommand())
	jaasCmd.Register(cmd.NewListControllersCommand())
	jaasCmd.Register(cmd.NewListJobsCommand())
	jaasCmd.Register(cmd.NewListGroupsCommand())
	jaasCmd.Register(cmd.NewListMigrationTargetsCommand())
	jaasCmd.Register(cmd.NewListPermissionsCommand())
	jaasCmd.Register(cmd.NewListRolesCommand())
	jaasCmd.Register(cmd.NewMigrateModelCommand())
	jaasCmd.Register(cmd.NewMigrateInternalModelCommand())
	jaasCmd.Register(cmd.NewModelStatusCommand())
	jaasCmd.Register(cmd.NewPurgeLogsCommand())
	jaasCmd.Register(cmd.NewRegisterControllerCommand())
	jaasCmd.Register(cmd.NewRemoveCloudFromControllerCommand())
	jaasCmd.Register(cmd.NewRemoveControllerProfileCommand())
	jaasCmd.Register(cmd.NewRemovePermissionCommand())
	jaasCmd.Register(cmd.NewRemoveGroupCommand())
	jaasCmd.Register(cmd.NewRemoveRoleCommand())
	jaasCmd.Register(cmd.NewRenameGroupCommand())
	jaasCmd.Register(cmd.NewRenameRoleCommand())
	jaasCmd.Register(cmd.NewRevokeAuditLogAccessCommand())
	jaasCmd.Register(cmd.NewSetControllerDeprecatedCommand())
	jaasCmd.Register(cmd.NewShowControllerCommand())
	jaasCmd.Register(cmd.NewShowControllerProfileCommand())
	jaasCmd.Register(cmd.NewShowModelCommand())
	jaasCmd.Register(cmd.NewUnregisterControllerCommand())
	jaasCmd.Register(cmd.NewUpdateControllerProfileCommand())
	jaasCmd.Register(cmd.NewUpdateMigratedModelCommand())
	jaasCmd.Register(cmd.NewUpgradeToCommand())
	jaasCmd.Register(cmd.NewBootstrapStatusCommand())
	jaasCmd.Register(cmd.NewBootstrapStopCommand())
	jaasCmd.Register(cmd.NewBootstrapStartCommand())
	jaasCmd.Register(cmd.NewDestroyControllerStartCommand())
	return jaasCmd
}

const (
	jujuPrefix  = "juju-"
	jaasCommand = "juju-jaas"
)

func main() {
	ctx, err := jujucmd.DefaultContext()
	if err != nil {
		fmt.Printf("failed to get command context: %v\n", err)
		os.Exit(2)
	}
	superCmd := NewSuperCommand()
	var args []string
	// The following if condition handles cases where the juju binary calls jaas as a plugin.
	// Symlinks of the form juju-<command> are created to make all jaas commands appear as top
	// level commands to the Juju CLI and then we strip the juju- prefix to obtain the desired function.
	if strings.HasPrefix(os.Args[0], jujuPrefix) && os.Args[0] != jaasCommand {
		args = make([]string, len(os.Args))
		copy(args[1:], os.Args[1:])
		args[0] = strings.TrimPrefix(os.Args[0], "juju-")
	} else {
		args = os.Args[1:]
	}
	os.Exit(jujucmd.Main(superCmd, ctx, args))
}
