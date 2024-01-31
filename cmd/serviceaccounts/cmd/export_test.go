// Copyright 2021 Canonical Ltd.

package cmd

import (
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/httpbakery"
	"github.com/juju/cmd/v3"
	jujuapi "github.com/juju/juju/api"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/jujuclient"
)

func NewAddServiceAccountCommandForTesting(store jujuclient.ClientStore, bClient *httpbakery.Client) cmd.Command {
	cmd := &addServiceAccountCommand{
		store: store,
		dialOpts: &jujuapi.DialOpts{
			InsecureSkipVerify: true,
			BakeryClient:       bClient,
		},
	}

	return modelcmd.WrapBase(cmd)
}

func NewListServiceAccountCredentialsCommandForTesting(store jujuclient.ClientStore, bClient *httpbakery.Client) cmd.Command {
	cmd := &listServiceAccountCredentialsCommand{
		store: store,
		dialOpts: &jujuapi.DialOpts{
			InsecureSkipVerify: true,
			BakeryClient:       bClient,
		},
	}

	return modelcmd.WrapBase(cmd)
}
