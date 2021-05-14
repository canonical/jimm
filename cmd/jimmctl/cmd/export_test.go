// Copyright 2021 Canonical Ltd.

package cmd

import (
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/httpbakery"
	"github.com/juju/cmd"
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
