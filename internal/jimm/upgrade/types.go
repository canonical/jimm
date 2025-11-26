// Copyright 2025 Canonical.

package upgrade

import (
	jujucloud "github.com/juju/juju/cloud"
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
