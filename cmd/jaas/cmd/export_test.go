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
	cmd := &listControllersCommand{}
	cmd.SetClientStore(store)

	return modelcmd.WrapBase(cmd)
}

func NewModelStatusCommandForTesting(store jujuclient.ClientStore, lp jujuapi.LoginProvider) cmd.Command {
	cmd := &modelStatusCommand{
		dialOpts: cmdtest.TestDialOpts(lp),
	}
	cmd.SetClientStore(store)

	return modelcmd.WrapBase(cmd)
}

func NewRevokeAuditLogAccessCommandForTesting(store jujuclient.ClientStore, lp jujuapi.LoginProvider) cmd.Command {
	cmd := &revokeAuditLogAccessCommand{
		dialOpts: cmdtest.TestDialOpts(lp),
	}
	cmd.SetClientStore(store)

	return modelcmd.WrapBase(cmd)
}

func NewSetControllerDeprecatedCommandForTesting(store jujuclient.ClientStore, lp jujuapi.LoginProvider) cmd.Command {
	cmd := &setControllerDeprecatedCommand{
		dialOpts: cmdtest.TestDialOpts(lp),
	}
	cmd.SetClientStore(store)

	return modelcmd.WrapBase(cmd)
}

func NewAddGroupCommandForTesting(store jujuclient.ClientStore, lp jujuapi.LoginProvider) cmd.Command {
	cmd := &addGroupCommand{
		dialOpts: cmdtest.TestDialOpts(lp),
	}
	cmd.SetClientStore(store)

	return modelcmd.WrapBase(cmd)
}

func NewCrossModelQueryCommandForTesting(store jujuclient.ClientStore, lp jujuapi.LoginProvider) cmd.Command {
	cmd := &crossModelQueryCommand{
		dialOpts: cmdtest.TestDialOpts(lp),
	}
	cmd.SetClientStore(store)
	cmd.crossModelQueryAPIFunc = cmd.newClient

	return modelcmd.WrapBase(cmd)
}

func NewMigrateModelCommandForTesting(store jujuclient.ClientStore, lp jujuapi.LoginProvider) *migrateModelCommand {
	cmd := &migrateModelCommand{
		dialOpts: cmdtest.TestDialOpts(lp),
	}
	cmd.SetClientStore(store)
	return cmd
}

func NewUpgradeToCommandForTesting(store jujuclient.ClientStore, lp jujuapi.LoginProvider) *upgradeToCommand {
	cmd := &upgradeToCommand{
		dialOpts: cmdtest.TestDialOpts(lp),
	}
	cmd.SetClientStore(store)
	return cmd
}
