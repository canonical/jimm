// Copyright 2024 Canonical Ltd.

package cmd

import (
	"github.com/juju/cmd/v3"
	jujuapi "github.com/juju/juju/api"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/jujuclient"
)

func NewAddServiceAccountCommandForTesting(store jujuclient.ClientStore, lp jujuapi.LoginProvider) cmd.Command {
	cmd := &addServiceAccountCommand{
		store: store,
		dialOpts: &jujuapi.DialOpts{
			InsecureSkipVerify: true,
			LoginProvider:      lp,
		},
	}

	return modelcmd.WrapBase(cmd)
}

func NewAddServiceAccountCredentialCommandForTesting(store jujuclient.ClientStore, lp jujuapi.LoginProvider) cmd.Command {
	cmd := &addServiceAccountCredentialCommand{
		store: store,
		dialOpts: &jujuapi.DialOpts{
			InsecureSkipVerify: true,
			LoginProvider:      lp,
		},
	}

	return modelcmd.WrapBase(cmd)
}

func NewListServiceAccountCredentialsCommandForTesting(store jujuclient.ClientStore, lp jujuapi.LoginProvider) cmd.Command {
	cmd := &listServiceAccountCredentialsCommand{
		store: store,
		dialOpts: &jujuapi.DialOpts{
			InsecureSkipVerify: true,
			LoginProvider:      lp,
		},
	}

	return modelcmd.WrapBase(cmd)
}

func NewUpdateCredentialsCommandForTesting(store jujuclient.ClientStore, lp jujuapi.LoginProvider) cmd.Command {
	cmd := &updateCredentialCommand{
		store: store,
		dialOpts: &jujuapi.DialOpts{
			InsecureSkipVerify: true,
			LoginProvider:      lp,
		},
	}

	return modelcmd.WrapBase(cmd)
}

func NewGrantCommandForTesting(store jujuclient.ClientStore, lp jujuapi.LoginProvider) cmd.Command {
	cmd := &grantCommand{
		store: store,
		dialOpts: &jujuapi.DialOpts{
			InsecureSkipVerify: true,
			LoginProvider:      lp,
		},
	}

	return modelcmd.WrapBase(cmd)
}
