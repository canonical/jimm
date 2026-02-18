// Copyright 2025 Canonical.

package bootstrap

import (
	"fmt"
	"strings"
	"time"

	jujucloud "github.com/juju/juju/cloud"

	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/rivertypes"
)

// BootstrapParams defines the parameters required for bootstrapping a JIMM controller.
type BootstrapParams struct {
	CLIVersion string

	CloudNameAndRegion string
	ControllerName     string

	CloudCred jujucloud.Credential
	Cloud     jujucloud.Cloud

	UserConfig map[string]string
}

// RunnerArgs defines the parameters required for running a bootstrap or destroy-controller job.
type RunnerArgs struct {
	JujuDataDir string
	JobID       int64
}

// RunBootstrapArgs combines the arguments for the bootstrap worker,
// including both the Rivertypes and the RunnerArgs.
type RunBootstrapArgs struct {
	rivertypes.BootstrapArgs
	RunnerArgs
}

// RunDestroyControllerArgs combines the arguments for the destroy-controller worker,
// including both the Rivertypes and the RunnerArgs.
type RunDestroyControllerArgs struct {
	rivertypes.DestroyControllerArgs
	RunnerArgs
}

// WaitConfig holds the configuration for waiting for job completion.
type WaitConfig struct {
	// MaxJobDuration is the maximum duration to wait for a job to complete.
	MaxJobDuration time.Duration
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
	ControllerUUID string
	AgentVersion   string
	CloudName      string
	CloudRegion    string
	APIEndpoints   []string
	PublicAddress  string
	CACertificate  string
}
