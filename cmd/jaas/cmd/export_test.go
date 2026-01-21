// Copyright 2026 Canonical.

package cmd

import (
	"github.com/juju/cmd/v3"
	jujuapi "github.com/juju/juju/api"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/jujuclient"

	"github.com/canonical/jimm/v3/internal/testutils/cmdtest"
)

var (
	AccessMessage          = accessMessageFormat
	AccessResultAllowed    = accessResultAllowed
	AccessResultDenied     = accessResultDenied
	DefaultPageSize        = defaultPageSize
	FormatRelationsTabular = formatRelationsTabular
)

type AccessResult = accessResult

func NewListControllersCommandForTesting(store jujuclient.ClientStore, lp jujuapi.LoginProvider) cmd.Command {
	cmd := &listControllersCommand{
		store: store,
	}

	return modelcmd.WrapBase(cmd)
}

func NewModelStatusCommandForTesting(store jujuclient.ClientStore, lp jujuapi.LoginProvider) cmd.Command {
	cmd := &modelStatusCommand{
		store:    store,
		dialOpts: cmdtest.TestDialOpts(lp),
	}

	return modelcmd.WrapBase(cmd)
}

func NewGrantAuditLogAccessCommandForTesting(store jujuclient.ClientStore, lp jujuapi.LoginProvider) cmd.Command {
	cmd := &grantAuditLogAccessCommand{
		store:    store,
		dialOpts: cmdtest.TestDialOpts(lp),
	}

	return modelcmd.WrapBase(cmd)
}

func NewRevokeAuditLogAccessCommandForTesting(store jujuclient.ClientStore, lp jujuapi.LoginProvider) cmd.Command {
	cmd := &revokeAuditLogAccessCommand{
		store:    store,
		dialOpts: cmdtest.TestDialOpts(lp),
	}

	return modelcmd.WrapBase(cmd)
}

func NewListAuditEventsCommandForTesting(store jujuclient.ClientStore, lp jujuapi.LoginProvider) cmd.Command {
	cmd := &listAuditEventsCommand{
		store:    store,
		dialOpts: cmdtest.TestDialOpts(lp),
	}

	return modelcmd.WrapBase(cmd)
}

type RemoveCloudFromControllerAPI = removeCloudFromControllerAPI

func NewRemoveCloudFromControllerCommandForTesting(store jujuclient.ClientStore, lp jujuapi.LoginProvider, removeCloudFromControllerAPIFunc func() (RemoveCloudFromControllerAPI, error)) cmd.Command {
	cmd := &removeCloudFromControllerCommand{
		store:                            store,
		dialOpts:                         cmdtest.TestDialOpts(lp),
		removeCloudFromControllerAPIFunc: removeCloudFromControllerAPIFunc,
	}
	if removeCloudFromControllerAPIFunc == nil {
		cmd.removeCloudFromControllerAPIFunc = cmd.cloudAPI
	}

	return modelcmd.WrapBase(cmd)
}

func NewRegisterControllerCommandForTesting(store jujuclient.ClientStore, lp jujuapi.LoginProvider) cmd.Command {
	cmd := &registerControllerCommand{
		store:    store,
		dialOpts: cmdtest.TestDialOpts(lp),
	}

	return modelcmd.WrapBase(cmd)
}

func NewUnregisterControllerCommandForTesting(store jujuclient.ClientStore, lp jujuapi.LoginProvider) cmd.Command {
	cmd := &unregisterControllerCommand{
		store:    store,
		dialOpts: cmdtest.TestDialOpts(lp),
	}

	return modelcmd.WrapBase(cmd)
}

func NewSetControllerDeprecatedCommandForTesting(store jujuclient.ClientStore, lp jujuapi.LoginProvider) cmd.Command {
	cmd := &setControllerDeprecatedCommand{
		store:    store,
		dialOpts: cmdtest.TestDialOpts(lp),
	}

	return modelcmd.WrapBase(cmd)
}

func NewImportModelCommandForTesting(store jujuclient.ClientStore, lp jujuapi.LoginProvider) cmd.Command {
	cmd := &importModelCommand{
		store:    store,
		dialOpts: cmdtest.TestDialOpts(lp),
	}

	return modelcmd.WrapBase(cmd)
}

func NewUpdateMigratedModelCommandForTesting(store jujuclient.ClientStore, lp jujuapi.LoginProvider) cmd.Command {
	cmd := &updateMigratedModelCommand{
		store:    store,
		dialOpts: cmdtest.TestDialOpts(lp),
	}

	return modelcmd.WrapBase(cmd)
}

func NewAddRoleCommandForTesting(store jujuclient.ClientStore, lp jujuapi.LoginProvider) cmd.Command {
	cmd := &addRoleCommand{
		store:    store,
		dialOpts: cmdtest.TestDialOpts(lp),
	}

	return modelcmd.WrapBase(cmd)
}

func NewRenameRoleCommandForTesting(store jujuclient.ClientStore, lp jujuapi.LoginProvider) cmd.Command {
	cmd := &renameRoleCommand{
		store:    store,
		dialOpts: cmdtest.TestDialOpts(lp),
	}

	return modelcmd.WrapBase(cmd)
}

func NewRemoveRoleCommandForTesting(store jujuclient.ClientStore, lp jujuapi.LoginProvider) cmd.Command {
	cmd := &removeRoleCommand{
		store:    store,
		dialOpts: cmdtest.TestDialOpts(lp),
	}

	return modelcmd.WrapBase(cmd)
}

func NewListRolesCommandForTesting(store jujuclient.ClientStore, lp jujuapi.LoginProvider) cmd.Command {
	cmd := &listRolesCommand{
		store:    store,
		dialOpts: cmdtest.TestDialOpts(lp),
	}

	return modelcmd.WrapBase(cmd)
}

func NewAddGroupCommandForTesting(store jujuclient.ClientStore, lp jujuapi.LoginProvider) cmd.Command {
	cmd := &addGroupCommand{
		store:    store,
		dialOpts: cmdtest.TestDialOpts(lp),
	}

	return modelcmd.WrapBase(cmd)
}

func NewAddRelationCommandForTesting(store jujuclient.ClientStore, lp jujuapi.LoginProvider) cmd.Command {
	cmd := &addPermission{
		store:    store,
		dialOpts: cmdtest.TestDialOpts(lp),
	}

	return modelcmd.WrapBase(cmd)
}

func NewRemovePermissionCommandForTesting(store jujuclient.ClientStore, lp jujuapi.LoginProvider) cmd.Command {
	cmd := &removePermissionCommand{
		store:    store,
		dialOpts: cmdtest.TestDialOpts(lp),
	}

	return modelcmd.WrapBase(cmd)
}

func NewListPermissionsCommandForTesting(store jujuclient.ClientStore, lp jujuapi.LoginProvider) cmd.Command {
	cmd := &listPermissionsCommand{
		store:    store,
		dialOpts: cmdtest.TestDialOpts(lp),
	}

	return modelcmd.WrapBase(cmd)
}

func NewCheckPermissionCommandForTesting(store jujuclient.ClientStore, lp jujuapi.LoginProvider) cmd.Command {
	cmd := &checkPermissionCommand{
		store:    store,
		dialOpts: cmdtest.TestDialOpts(lp),
	}

	return modelcmd.WrapBase(cmd)
}

func NewCrossModelQueryCommandForTesting(store jujuclient.ClientStore, lp jujuapi.LoginProvider) cmd.Command {
	cmd := &crossModelQueryCommand{
		store:    store,
		dialOpts: cmdtest.TestDialOpts(lp),
	}
	cmd.crossModelQueryAPIFunc = cmd.newClient

	return modelcmd.WrapBase(cmd)
}

func NewPurgeLogsCommandForTesting(store jujuclient.ClientStore, lp jujuapi.LoginProvider) cmd.Command {
	cmd := &purgeLogsCommand{
		store:    store,
		dialOpts: cmdtest.TestDialOpts(lp),
	}

	return modelcmd.WrapBase(cmd)
}

func NewMigrateModelCommandForTesting(store jujuclient.ClientStore, lp jujuapi.LoginProvider) *migrateModelCommand {
	return &migrateModelCommand{
		store:    store,
		dialOpts: cmdtest.TestDialOpts(lp),
	}
}

func NewUpgradeToCommandForTesting(store jujuclient.ClientStore, lp jujuapi.LoginProvider) *upgradeToCommand {
	return &upgradeToCommand{
		store:    store,
		dialOpts: cmdtest.TestDialOpts(lp),
	}
}
