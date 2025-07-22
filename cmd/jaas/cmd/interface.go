// Copyright 2025 Canonical.

package cmd

import "github.com/canonical/jimm/v3/pkg/api/params"

// JIMMClient is an interface that defines the methods required for JIMM client operations.
type JIMMClient interface {
	BootstrapStatus(req *params.BootstrapStatusRequest) (params.BootstrapStatusResponse, error)
	Bootstrap(req *params.BootstrapStartParams) (*params.BootstrapStartResponse, error)
}
