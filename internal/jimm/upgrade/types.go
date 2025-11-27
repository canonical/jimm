// Copyright 2025 Canonical.

package upgrade

import (
	"context"

	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/openfga"
	"github.com/juju/juju/api/base"
	jujucloud "github.com/juju/juju/cloud"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v5"
)

// CloneControllerParams defines the parameters required for bootstrapping a Juju controller.
type CloneControllerParams struct {
	CLIVersion string

	CloudNameAndRegion string
	ControllerName     string

	CloudCred jujucloud.Credential
	// PersonalCloud is the cloud-definition for a non-public cloud.
	PersonalCloud jujucloud.Cloud

	UserConfig map[string]string
}

// A Dialer provides a connection to a controller.
type Dialer interface {
	// Dial creates an API connection to a controller. If the given
	// model-tag is non-zero the connection will be to that model,
	// otherwise the connection is to the controller. After successfully
	// dialing the controller the UUID, AgentVersion and HostPorts fields
	// in the given controller should be updated to the values provided
	// by the controller.
	Dial(ctx context.Context, ctl *dbmodel.Controller, modelTag names.ModelTag, user *openfga.User, withPermissions map[string]string) (UpgradeManagerAPI, error)
}

// UpgradeManagerAPI defines the subset of the Juju API used by the UpgradeManager.
type UpgradeManagerAPI interface {
	// API implements the base.APICallCloser so that we can
	// use the juju api clients to interact with juju controllers.
	base.APICallCloser

	// ControllerModelSummary fetches the model summary of the model on the
	// controller that hosts the controller machines.
	ControllerModelSummary(context.Context, *jujuparams.ModelSummary) error
}
