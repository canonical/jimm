// Copyright 2025 Canonical.

package bootstrap

import (
	"fmt"
	"strings"

	jujucloud "github.com/juju/juju/cloud"
	semversion "github.com/juju/version"

	"github.com/canonical/jimm/v3/internal/errors"
)

// BootstrapParams defines the parameters required for bootstrapping a JIMM controller.
type BootstrapParams struct {
	CLIVersion string

	CloudNameAndRegion string
	ControllerName     string
	AgentVersion       string
	BootstrapTimeout   int

	CloudCred jujucloud.CloudCredential
	// PersonalCloud is the cloud-definition for a non-public cloud.
	PersonalCloud jujucloud.Cloud
}

// Validate checks if the BootstrapParams are valid.
func (p BootstrapParams) validate() error {
	var msgs []string
	if p.CLIVersion == "" {
		msgs = append(msgs, "CLI version cannot be empty")
	}
	if p.CloudNameAndRegion == "" {
		msgs = append(msgs, "cloud name and region cannot be empty")
	}
	if p.ControllerName == "" {
		msgs = append(msgs, "controller name cannot be empty")
	}
	if p.AgentVersion != "" {
		if _, err := semversion.Parse(p.AgentVersion); err != nil {
			msgs = append(msgs, fmt.Sprintf("invalid agent version: %v", err))
		}
	}
	if p.BootstrapTimeout < 0 {
		msgs = append(msgs, "bootstrap timeout cannot be less than zero")
	}

	// Don't validate cloud or cloud cred. Juju knows better how to validate those.
	// And will return a better validation error if they are invalid.

	if len(msgs) == 0 {
		return nil
	}

	// If there are validation errors, return them as a single error.
	if msgs != nil {
		return errors.E(fmt.Sprintf("invalid bootstrap parameters:\n%s", strings.Join(msgs, "\n")))
	}
	return nil
}
