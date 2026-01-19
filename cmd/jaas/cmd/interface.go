// Copyright 2025 Canonical.

package cmd

import "github.com/canonical/jimm/v3/pkg/api/params"

// JIMMAPI is an interface that defines the methods required for JIMM client operations.
type JIMMAPI interface {
	Close() error
	CrossModelQuery(req *params.CrossModelQueryRequest) (*params.CrossModelQueryResponse, error)
	GetJobInfo(req *params.GetJobInfoRequest) (params.GetJobInfoResponse, error)
	StopJob(req *params.StopJobRequest) error
	StartBootstrapJob(req *params.BootstrapParams) (*params.StartJobResponse, error)
	StartDestroyControllerJob(req *params.DestroyControllerRequest) (*params.StartJobResponse, error)
	ListMigrationTargets(req *params.ListMigrationTargetsRequest) ([]params.ControllerInfo, error)
	PrepareModelMigration(req *params.PrepareModelMigrationRequest) (params.PrepareModelMigrationResponse, error)
	UpgradeTo(req *params.UpgradeToRequest) (params.UpgradeToResponse, error)
}
