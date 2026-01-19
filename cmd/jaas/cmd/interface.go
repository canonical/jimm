// Copyright 2025 Canonical.

package cmd

import (
	"github.com/canonical/jimm/v3/pkg/api/params"
	jujuparams "github.com/juju/juju/rpc/params"
)

// JIMMAPI is an interface that defines the methods required for JIMM client operations.
type JIMMAPI interface {
	Close() error

	// Job operations
	GetJobInfo(req *params.GetJobInfoRequest) (params.GetJobInfoResponse, error)
	StopJob(req *params.StopJobRequest) error
	StartBootstrapJob(req *params.BootstrapParams) (*params.StartJobResponse, error)
	StartDestroyControllerJob(req *params.DestroyControllerRequest) (*params.StartJobResponse, error)

	// Controller operations
	AddCloudToController(req *params.AddCloudToControllerRequest) error
	AddController(req *params.AddControllerRequest) (params.ControllerInfo, error)
	ListControllers() ([]params.ControllerInfo, error)
	RemoveCloudFromController(req *params.RemoveCloudFromControllerRequest) error
	RemoveController(req *params.RemoveControllerRequest) (params.ControllerInfo, error)
	SetControllerDeprecated(req *params.SetControllerDeprecatedRequest) (params.ControllerInfo, error)

	// Migration operations
	ListMigrationTargets(req *params.ListMigrationTargetsRequest) ([]params.ControllerInfo, error)
	PrepareModelMigration(req *params.PrepareModelMigrationRequest) (params.PrepareModelMigrationResponse, error)
	MigrateModel(req *params.MigrateModelRequest) (*jujuparams.InitiateMigrationResults, error)
	ImportModel(req *params.ImportModelRequest) error
	UpdateMigratedModel(req *params.UpdateMigratedModelRequest) error

	// Model operations
	FullModelStatus(req *params.FullModelStatusRequest) (jujuparams.FullStatus, error)

	// Audit log operations
	FindAuditEvents(req *params.FindAuditEventsRequest) (params.AuditEvents, error)
	GrantAuditLogAccess(req *params.AuditLogAccessRequest) error
	RevokeAuditLogAccess(req *params.AuditLogAccessRequest) error
	PurgeLogs(req *params.PurgeLogsRequest) (*params.PurgeLogsResponse, error)

	// Group operations
	AddGroup(req *params.AddGroupRequest) (params.AddGroupResponse, error)
	GetGroup(req *params.GetGroupRequest) (params.GetGroupResponse, error)
	RenameGroup(req *params.RenameGroupRequest) error
	RemoveGroup(req *params.RemoveGroupRequest) error
	ListGroups(req *params.ListGroupsRequest) ([]params.Group, error)

	// Role operations
	AddRole(req *params.AddRoleRequest) (params.AddRoleResponse, error)
	GetRole(req *params.GetRoleRequest) (params.GetRoleResponse, error)
	RenameRole(req *params.RenameRoleRequest) error
	RemoveRole(req *params.RemoveRoleRequest) error
	ListRoles(req *params.ListRolesRequest) ([]params.Role, error)

	// Permission operations
	AddRelation(req *params.AddRelationRequest) error
	RemoveRelation(req *params.RemoveRelationRequest) error
	CheckRelation(req *params.CheckRelationRequest) (params.CheckRelationResponse, error)
	ListRelationshipTuples(req *params.ListRelationshipTuplesRequest) (*params.ListRelationshipTuplesResponse, error)

	// Query operations
	CrossModelQuery(req *params.CrossModelQueryRequest) (*params.CrossModelQueryResponse, error)

	// Other operations
	UpgradeTo(req *params.UpgradeToRequest) (params.UpgradeToResponse, error)
}
