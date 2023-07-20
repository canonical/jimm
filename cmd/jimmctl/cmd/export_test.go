// Copyright 2021 Canonical Ltd.

package cmd

import (
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/httpbakery"
	"github.com/juju/cmd/v3"
	jujuapi "github.com/juju/juju/api"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/jujuclient"
)

var (
	AccessMessage       = accessMessageFormat
	AccessResultAllowed = accessResultAllowed
	AccessResultDenied  = accessResultDenied
)

type AccessResult = accessResult

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

func NewAddCloudToControllerCommandForTesting(store jujuclient.ClientStore, bClient *httpbakery.Client, cloudByNameFunc func(string) (*cloud.Cloud, error)) cmd.Command {
	cmd := &addCloudToControllerCommand{
		store:           store,
		cloudByNameFunc: cloudByNameFunc,
		dialOpts: &jujuapi.DialOpts{
			InsecureSkipVerify: true,
			BakeryClient:       bClient,
		},
	}

	return modelcmd.WrapBase(cmd)
}

type RemoveCloudFromControllerAPI = removeCloudFromControllerAPI

func NewRemoveCloudFromControllerCommandForTesting(store jujuclient.ClientStore, bClient *httpbakery.Client, removeCloudFromControllerAPIFunc func() (RemoveCloudFromControllerAPI, error)) cmd.Command {
	cmd := &removeCloudFromControllerCommand{
		store: store,
		dialOpts: &jujuapi.DialOpts{
			InsecureSkipVerify: true,
			BakeryClient:       bClient,
		},
		removeCloudFromControllerAPIFunc: removeCloudFromControllerAPIFunc,
	}
	if removeCloudFromControllerAPIFunc == nil {
		cmd.removeCloudFromControllerAPIFunc = cmd.cloudAPI
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

func NewUpdateMigratedModelCommandForTesting(store jujuclient.ClientStore, bClient *httpbakery.Client) cmd.Command {
	cmd := &updateMigratedModelCommand{
		store: store,
		dialOpts: &jujuapi.DialOpts{
			InsecureSkipVerify: true,
			BakeryClient:       bClient,
		},
	}

	return modelcmd.WrapBase(cmd)
}

func NewImportCloudCredentialsCommandForTesting(store jujuclient.ClientStore, bClient *httpbakery.Client) cmd.Command {
	cmd := &importCloudCredentialsCommand{
		store: store,
		dialOpts: &jujuapi.DialOpts{
			InsecureSkipVerify: true,
			BakeryClient:       bClient,
		},
	}

	return modelcmd.WrapBase(cmd)
}

func NewAddGroupCommandForTesting(store jujuclient.ClientStore, bClient *httpbakery.Client) cmd.Command {
	cmd := &addGroupCommand{
		store: store,
		dialOpts: &jujuapi.DialOpts{
			InsecureSkipVerify: true,
			BakeryClient:       bClient,
		},
	}

	return modelcmd.WrapBase(cmd)
}

func NewRenameGroupCommandForTesting(store jujuclient.ClientStore, bClient *httpbakery.Client) cmd.Command {
	cmd := &renameGroupCommand{
		store: store,
		dialOpts: &jujuapi.DialOpts{
			InsecureSkipVerify: true,
			BakeryClient:       bClient,
		},
	}

	return modelcmd.WrapBase(cmd)
}

func NewRemoveGroupCommandForTesting(store jujuclient.ClientStore, bClient *httpbakery.Client) cmd.Command {
	cmd := &removeGroupCommand{
		store: store,
		dialOpts: &jujuapi.DialOpts{
			InsecureSkipVerify: true,
			BakeryClient:       bClient,
		},
	}

	return modelcmd.WrapBase(cmd)
}

func NewListGroupsCommandForTesting(store jujuclient.ClientStore, bClient *httpbakery.Client) cmd.Command {
	cmd := &listGroupsCommand{
		store: store,
		dialOpts: &jujuapi.DialOpts{
			InsecureSkipVerify: true,
			BakeryClient:       bClient,
		},
	}

	return modelcmd.WrapBase(cmd)
}

func NewAddRelationCommandForTesting(store jujuclient.ClientStore, bClient *httpbakery.Client) cmd.Command {
	cmd := &addRelationCommand{
		store: store,
		dialOpts: &jujuapi.DialOpts{
			InsecureSkipVerify: true,
			BakeryClient:       bClient,
		},
	}

	return modelcmd.WrapBase(cmd)
}

func NewRemoveRelationCommandForTesting(store jujuclient.ClientStore, bClient *httpbakery.Client) cmd.Command {
	cmd := &removeRelationCommand{
		store: store,
		dialOpts: &jujuapi.DialOpts{
			InsecureSkipVerify: true,
			BakeryClient:       bClient,
		},
	}

	return modelcmd.WrapBase(cmd)
}

func NewListRelationsCommandForTesting(store jujuclient.ClientStore, bClient *httpbakery.Client) cmd.Command {
	cmd := &listRelationsCommand{
		store: store,
		dialOpts: &jujuapi.DialOpts{
			InsecureSkipVerify: true,
			BakeryClient:       bClient,
		},
	}

	return modelcmd.WrapBase(cmd)
}

func NewCheckRelationCommandForTesting(store jujuclient.ClientStore, bClient *httpbakery.Client) cmd.Command {
	cmd := &checkRelationCommand{
		store: store,
		dialOpts: &jujuapi.DialOpts{
			InsecureSkipVerify: true,
			BakeryClient:       bClient,
		},
	}

	return modelcmd.WrapBase(cmd)
}

func NewCrossModelQueryCommandForTesting(store jujuclient.ClientStore, bClient *httpbakery.Client) cmd.Command {
	cmd := &crossModelQueryCommand{
		store: store,
		dialOpts: &jujuapi.DialOpts{
			InsecureSkipVerify: true,
			BakeryClient:       bClient,
		},
	}

	return modelcmd.WrapBase(cmd)
}

func NewPurgeLogsCommandForTesting(store jujuclient.ClientStore, bClient *httpbakery.Client) cmd.Command {
	cmd := &purgeLogsCommand{
		store: store,
		dialOpts: &jujuapi.DialOpts{
			InsecureSkipVerify: true,
			BakeryClient:       bClient,
		},
	}

	return modelcmd.WrapBase(cmd)
}