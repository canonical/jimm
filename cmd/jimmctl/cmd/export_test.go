// Copyright 2024 Canonical.

package cmd

import (
	"github.com/juju/cmd/v3"
	jujuapi "github.com/juju/juju/api"
	"github.com/juju/juju/cloud"
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
		store:    store,
		dialOpts: cmdtest.TestDialOpts(lp),
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

func NewAddCloudToControllerCommandForTesting(store jujuclient.ClientStore, lp jujuapi.LoginProvider, cloudByNameFunc func(string) (*cloud.Cloud, error)) cmd.Command {
	cmd := &addCloudToControllerCommand{
		store:           store,
		cloudByNameFunc: cloudByNameFunc,
		dialOpts:        cmdtest.TestDialOpts(lp),
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

func NewAddControllerCommandForTesting(store jujuclient.ClientStore, lp jujuapi.LoginProvider) cmd.Command {
	cmd := &addControllerCommand{
		store:    store,
		dialOpts: cmdtest.TestDialOpts(lp),
	}

	return modelcmd.WrapBase(cmd)
}

func NewRemoveControllerCommandForTesting(store jujuclient.ClientStore, lp jujuapi.LoginProvider) cmd.Command {
	cmd := &removeControllerCommand{
		store:    store,
		dialOpts: cmdtest.TestDialOpts(lp),
	}

	return modelcmd.WrapBase(cmd)
}

func NewControllerInfoCommandForTesting(store jujuclient.ClientStore) cmd.Command {
	cmd := &controllerInfoCommand{
		store: store,
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

func NewImportCloudCredentialsCommandForTesting(store jujuclient.ClientStore, lp jujuapi.LoginProvider) cmd.Command {
	cmd := &importCloudCredentialsCommand{
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

func NewRenameGroupCommandForTesting(store jujuclient.ClientStore, lp jujuapi.LoginProvider) cmd.Command {
	cmd := &renameGroupCommand{
		store:    store,
		dialOpts: cmdtest.TestDialOpts(lp),
	}

	return modelcmd.WrapBase(cmd)
}

func NewRemoveGroupCommandForTesting(store jujuclient.ClientStore, lp jujuapi.LoginProvider) cmd.Command {
	cmd := &removeGroupCommand{
		store:    store,
		dialOpts: cmdtest.TestDialOpts(lp),
	}

	return modelcmd.WrapBase(cmd)
}

func NewListGroupsCommandForTesting(store jujuclient.ClientStore, lp jujuapi.LoginProvider) cmd.Command {
	cmd := &listGroupsCommand{
		store:    store,
		dialOpts: cmdtest.TestDialOpts(lp),
	}

	return modelcmd.WrapBase(cmd)
}

func NewAddRelationCommandForTesting(store jujuclient.ClientStore, lp jujuapi.LoginProvider) cmd.Command {
	cmd := &addRelationCommand{
		store:    store,
		dialOpts: cmdtest.TestDialOpts(lp),
	}

	return modelcmd.WrapBase(cmd)
}

func NewRemoveRelationCommandForTesting(store jujuclient.ClientStore, lp jujuapi.LoginProvider) cmd.Command {
	cmd := &removeRelationCommand{
		store:    store,
		dialOpts: cmdtest.TestDialOpts(lp),
	}

	return modelcmd.WrapBase(cmd)
}

func NewListRelationsCommandForTesting(store jujuclient.ClientStore, lp jujuapi.LoginProvider) cmd.Command {
	cmd := &listRelationsCommand{
		store:    store,
		dialOpts: cmdtest.TestDialOpts(lp),
	}

	return modelcmd.WrapBase(cmd)
}

func NewCheckRelationCommandForTesting(store jujuclient.ClientStore, lp jujuapi.LoginProvider) cmd.Command {
	cmd := &checkRelationCommand{
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

	return modelcmd.WrapBase(cmd)
}

func NewPurgeLogsCommandForTesting(store jujuclient.ClientStore, lp jujuapi.LoginProvider) cmd.Command {
	cmd := &purgeLogsCommand{
		store:    store,
		dialOpts: cmdtest.TestDialOpts(lp),
	}

	return modelcmd.WrapBase(cmd)
}

func NewMigrateModelCommandForTesting(store jujuclient.ClientStore, lp jujuapi.LoginProvider) cmd.Command {
	cmd := &migrateModelCommand{
		store:    store,
		dialOpts: cmdtest.TestDialOpts(lp),
	}

	return modelcmd.WrapBase(cmd)
}
