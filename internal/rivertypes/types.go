// Copyright 2026 Canonical.

package rivertypes

import (
	jujucloud "github.com/juju/juju/cloud"
	"github.com/juju/version/v2"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
)

// UpgradeToArgs are the arguments for the upgrade-to worker.
type UpgradeToArgs struct {
	ModelUUID            string         `json:"model-uuid" river:"unique"`
	TargetVersion        version.Number `json:"target-version"`
	Username             string         `json:"username"`
	TargetControllerName string         `json:"target_controller_name"`
}

// Kind implements the [river.JobArgs] interface.
func (UpgradeToArgs) Kind() string { return "upgrade-to" }

// InsertOpts implements the [river.JobArgsWithInsertOpts] interface.
func (UpgradeToArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		MaxAttempts: 3,
		UniqueOpts: river.UniqueOpts{
			ByArgs: true,
			ByState: []rivertype.JobState{
				rivertype.JobStateAvailable,
				rivertype.JobStatePending,
				rivertype.JobStateRunning,
				rivertype.JobStateRetryable,
				rivertype.JobStateScheduled,
			},
		},
	}
}

// BootstrapArgs are the arguments for the bootstrap-controller worker.
type BootstrapArgs struct {
	Username string `json:"username"`

	// CLI Download params.
	CLIVersion string `json:"cli-version"`

	// User defined command arguments
	CloudNameAndRegion string               `json:"cloud-name-and-region"`
	ControllerName     string               `json:"controller-name"`
	AgentVersion       string               `json:"agent-version"`
	CloudCred          jujucloud.Credential `json:"cloud-cred"`
	// Cloud contains the definition of the cloud e.g. endpoints, regions, TLS config.
	// It only needs to be set if the cloud is not a public cloud (e.g. not AWS, Azure, etc).
	Cloud jujucloud.Cloud `json:"cloud"`

	// JIMM Provided command arguments (i.e., ones that must be set by JIMM when bootstrapping).
	LoginTokenRefreshURL string `json:"login-token-refresh-url"`

	// User provided config
	UserConfig map[string]string `json:"user-config"`
}

// Kind implements the [river.JobArgs] interface.
func (BootstrapArgs) Kind() string { return "bootstrap-controller" }

// InsertOpts implements the [river.JobArgsWithInsertOpts] interface.
func (BootstrapArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		MaxAttempts: 1,
		// Only allow 1 bootstrap job at a time.
		// This is used in conjuction with a global mutex
		// in the bootstrap package to avoid issues with a
		// global lock in Juju's cmd pkg used during bootstrap/destroy.
		UniqueOpts: river.UniqueOpts{
			ByState: []rivertype.JobState{
				rivertype.JobStateAvailable,
				rivertype.JobStatePending,
				rivertype.JobStateRunning,
				rivertype.JobStateRetryable,
				rivertype.JobStateScheduled,
			},
		},
	}
}

// DestroyControllerArgs are the arguments for the destroy-controller worker.
type DestroyControllerArgs struct {
	Username       string   `json:"username"`
	ControllerName string   `json:"controller-name"`
	ControllerUUID string   `json:"controller-uuid"`
	AgentVersion   string   `json:"agent-version"`
	CloudName      string   `json:"cloud-name"`
	CloudRegion    string   `json:"cloud-region"`
	APIEndpoints   []string `json:"api-endpoints"`
	PublicAddress  string   `json:"public-address"`
	CACertificate  string   `json:"ca-certificate"`
}

// Kind implements the [river.JobArgs] interface.
func (DestroyControllerArgs) Kind() string { return "destroy-controller" }

// InsertOpts implements the [river.JobArgsWithInsertOpts] interface.
func (DestroyControllerArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		MaxAttempts: 1,
		UniqueOpts: river.UniqueOpts{
			// Only allow 1 destroy job at a time.
			// This is used in conjuction with a global mutex
			// in the bootstrap package to avoid issues with a
			// global lock in Juju's cmd pkg used during bootstrap/destroy.
			ByState: []rivertype.JobState{
				rivertype.JobStateAvailable,
				rivertype.JobStatePending,
				rivertype.JobStateRunning,
				rivertype.JobStateRetryable,
				rivertype.JobStateScheduled,
			},
		},
	}
}
