// Copyright 2026 Canonical.

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
	Cloud     jujucloud.Cloud

	UserConfig map[string]string
}
