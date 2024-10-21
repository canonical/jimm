// Copyright 2024 Canonical.

package cmd

import (
	"github.com/juju/cmd/v3"
	jujuapi "github.com/juju/juju/api"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/jujuclient"

	"github.com/canonical/jimm/v3/internal/testutils/cmdtest"
)

func NewAddServiceAccountCommandForTesting(store jujuclient.ClientStore, lp jujuapi.LoginProvider) cmd.Command {
	cmd := &addServiceAccountCommand{
		store:    store,
		dialOpts: cmdtest.TestDialOpts(lp),
	}

	return modelcmd.WrapBase(cmd)
}

func NewListServiceAccountCredentialsCommandForTesting(store jujuclient.ClientStore, lp jujuapi.LoginProvider) cmd.Command {
	cmd := &listServiceAccountCredentialsCommand{
		store:    store,
		dialOpts: cmdtest.TestDialOpts(lp),
	}

	return modelcmd.WrapBase(cmd)
}

func NewUpdateCredentialsCommandForTesting(store jujuclient.ClientStore, lp jujuapi.LoginProvider) cmd.Command {
	cmd := &updateCredentialCommand{
		store:    store,
		dialOpts: cmdtest.TestDialOpts(lp),
	}

	return modelcmd.WrapBase(cmd)
}

func NewGrantCommandForTesting(store jujuclient.ClientStore, lp jujuapi.LoginProvider) cmd.Command {
	cmd := &grantCommand{
		store:    store,
		dialOpts: cmdtest.TestDialOpts(lp),
	}

	return modelcmd.WrapBase(cmd)
}
