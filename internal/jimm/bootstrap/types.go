// Copyright 2025 Canonical.

package bootstrap

import (
	"fmt"
	"strings"

	"github.com/canonical/jimm/v3/internal/errors"
)

// BootstrapParams defines the parameters required for bootstrapping a JIMM controller.
type BootstrapParams struct {
	ControllerName string
	CloudName      string
	CloudRegion    string
	AgentVersion   string
	TimeoutSeconds int
}

// Validate checks if the BootstrapParams are valid.
func (p BootstrapParams) validate() error {
	var msgs []string
	if p.ControllerName == "" {
		msgs = append(msgs, "controller name cannot be empty")
	}
	if p.CloudName == "" {
		msgs = append(msgs, "cloud name cannot be empty")
	}
	if p.CloudRegion == "" {
		msgs = append(msgs, "cloud region cannot be empty")
	}
	if p.AgentVersion == "" {
		msgs = append(msgs, "agent version cannot be empty")
	}
	if msgs != nil {
		return errors.E(fmt.Sprintf("invalid bootstrap parameters:\n%s", strings.Join(msgs, "\n")))
	}
	return nil
}
