// Copyright 2025 Canonical.

package bootstrap

import (
	"fmt"
	"strings"

	jujucloud "github.com/juju/juju/cloud"

	"github.com/canonical/jimm/v3/internal/errors"
)

// BootstrapParams defines the parameters required for bootstrapping a JIMM controller.
type BootstrapParams struct {
	CLIVersion string

	CloudNameAndRegion string
	ControllerName     string

	CloudCred jujucloud.Credential
	// PersonalCloud is the cloud-definition for a non-public cloud.
	PersonalCloud jujucloud.Cloud

	UserConfig map[string]string
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

// DestroyControllerParams
type DestroyControllerParams struct {
	ControllerName string
	CLIVersion     string
}
