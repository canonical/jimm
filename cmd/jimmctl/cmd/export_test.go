// Copyright 2021 Canonical Ltd.

package cmd

import (
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/httpbakery"
	"github.com/juju/cmd/v3"
	jujuapi "github.com/juju/juju/api"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/jujuclient"
)

func NewListControllersCommandForTesting(store jujuclient.ClientStore, bClient *httpbakery.Client) cmd.Command {
	cmd := &listControllersCommand{
		store: store,
		dialOpts: &jujuapi.DialOpts{
			InsecureSkipVerify: true,
			BakeryClient:       bClient,
		},
	}

	return modelcmd.WrapBase(cmd)
}

func NewModelStatusCommandForTesting(store jujuclient.ClientStore, bClient *httpbakery.Client) cmd.Command {
	cmd := &modelStatusCommand{
		store: store,
		dialOpts: &jujuapi.DialOpts{
			InsecureSkipVerify: true,
			BakeryClient:       bClient,
		},
	}

	return modelcmd.WrapBase(cmd)
}

func NewGrantAuditLogAccessCommandForTesting(store jujuclient.ClientStore, bClient *httpbakery.Client) cmd.Command {
	cmd := &grantAuditLogAccessCommand{
		store: store,
		dialOpts: &jujuapi.DialOpts{
			InsecureSkipVerify: true,
			BakeryClient:       bClient,
		},
	}

	return modelcmd.WrapBase(cmd)
}

func NewRevokeAuditLogAccessCommandForTesting(store jujuclient.ClientStore, bClient *httpbakery.Client) cmd.Command {
	cmd := &revokeAuditLogAccessCommand{
		store: store,
		dialOpts: &jujuapi.DialOpts{
			InsecureSkipVerify: true,
			BakeryClient:       bClient,
		},
	}

	return modelcmd.WrapBase(cmd)
}

func NewListAuditEventsCommandForTesting(store jujuclient.ClientStore, bClient *httpbakery.Client) cmd.Command {
	cmd := &listAuditEventsCommand{
		store: store,
		dialOpts: &jujuapi.DialOpts{
			InsecureSkipVerify: true,
			BakeryClient:       bClient,
		},
	}

	return modelcmd.WrapBase(cmd)
}

func NewAddControllerCommandForTesting(store jujuclient.ClientStore, bClient *httpbakery.Client) cmd.Command {
	cmd := &addControllerCommand{
		store: store,
		dialOpts: &jujuapi.DialOpts{
			InsecureSkipVerify: true,
			BakeryClient:       bClient,
		},
	}

	return modelcmd.WrapBase(cmd)
}

func NewRemoveControllerCommandForTesting(store jujuclient.ClientStore, bClient *httpbakery.Client) cmd.Command {
	cmd := &removeControllerCommand{
		store: store,
		dialOpts: &jujuapi.DialOpts{
			InsecureSkipVerify: true,
			BakeryClient:       bClient,
		},
	}

	return modelcmd.WrapBase(cmd)
}

func NewControllerInfoCommandForTesting(store jujuclient.ClientStore) cmd.Command {
	cmd := &controllerInfoCommand{
		store: store,
	}

	return modelcmd.WrapBase(cmd)
}

func NewSetControllerDeprecatedCommandForTesting(store jujuclient.ClientStore, bClient *httpbakery.Client) cmd.Command {
	cmd := &setControllerDeprecatedCommand{
		store: store,
		dialOpts: &jujuapi.DialOpts{
			InsecureSkipVerify: true,
			BakeryClient:       bClient,
		},
	}

	return modelcmd.WrapBase(cmd)
}

func NewImportModelCommandForTesting(store jujuclient.ClientStore, bClient *httpbakery.Client) cmd.Command {
	cmd := &importModelCommand{
		store: store,
		dialOpts: &jujuapi.DialOpts{
			InsecureSkipVerify: true,
			BakeryClient:       bClient,
		},
	}

	return modelcmd.WrapBase(cmd)
}
