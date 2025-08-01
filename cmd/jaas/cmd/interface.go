// Copyright 2025 Canonical.

package cmd

import "github.com/canonical/jimm/v3/pkg/api/params"

// JIMMAPI is an interface that defines the methods required for JIMM client operations.
type JIMMAPI interface {
	Close() error
	BootstrapStatus(req *params.BootstrapStatusRequest) (params.BootstrapStatusResponse, error)
	Bootstrap(req *params.BootstrapStartParams) (*params.BootstrapStartResponse, error)
	BootstrapStop(req *params.BootstrapStopRequest) error
	PrepareModelMigration(req *params.PrepareModelMigrationRequest) (params.PrepareModelMigrationResponse, error)
}
