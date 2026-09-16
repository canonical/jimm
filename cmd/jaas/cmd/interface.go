// Copyright 2026 Canonical.

package cmd

import (
	"context"

	jujucloud "github.com/juju/juju/cloud"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v6"

	"github.com/canonical/jimm/v3/pkg/api/params"
)

// JIMMAPI is an interface that defines the methods required for JIMM client operations.
type JIMMAPI interface {
	Close() error

	// Job operations
	BootstrapInfo(ctx context.Context, req *params.GetBootstrapInfoRequest) (params.GetBootstrapInfoResponse, error)
	StopBootstrap(ctx context.Context, req *params.StopBootstrapRequest) error
	StartBootstrap(ctx context.Context, req *params.BootstrapParams) (*params.StartBootstrapResponse, error)
	StartDestroyController(ctx context.Context, req *params.DestroyControllerRequest) (*params.StartBootstrapResponse, error)

	// Cloud operations
	ListUserClouds(ctx context.Context, req *params.ListUserCloudsRequest) (map[names.CloudTag]jujucloud.Cloud, error)

	// Controller operations
	AddCloudToController(ctx context.Context, req *params.AddCloudToControllerRequest) error
	AddController(ctx context.Context, req *params.AddControllerRequest) (params.ControllerInfo, error)
	ListControllers(ctx context.Context) ([]params.ControllerInfo, error)
	RemoveCloudFromController(ctx context.Context, req *params.RemoveCloudFromControllerRequest) error
	RemoveController(ctx context.Context, req *params.RemoveControllerRequest) (params.ControllerInfo, error)
	SetControllerDeprecated(ctx context.Context, req *params.SetControllerDeprecatedRequest) (params.ControllerInfo, error)
	SaveControllerProfile(ctx context.Context, req *params.SaveControllerProfileRequest) (params.SaveControllerProfileResponse, error)
	GetControllerProfile(ctx context.Context, req *params.GetControllerProfileRequest) (params.GetControllerProfileResponse, error)
	ListControllerProfiles(ctx context.Context, req *params.ListControllerProfilesRequest) ([]params.ControllerProfileSummary, error)
	RemoveControllerProfile(ctx context.Context, req *params.RemoveControllerProfileRequest) error
	ShowController(ctx context.Context, controllerName string) (*params.ControllerDetails, error)

	// Migration operations
	ListMigrationTargets(ctx context.Context, req *params.ListMigrationTargetsRequest) ([]params.ControllerInfo, error)
	PrepareModelMigration(ctx context.Context, req *params.PrepareModelMigrationRequest) (params.PrepareModelMigrationResponse, error)
	MigrateModel(ctx context.Context, req *params.MigrateModelRequest) (*jujuparams.InitiateMigrationResults, error)
	ImportModel(ctx context.Context, req *params.ImportModelRequest) error
	RecoverModelCredential(ctx context.Context, req *params.RecoverModelCredentialRequest) (params.RecoverModelCredentialResponse, error)
	UpdateMigratedModel(ctx context.Context, req *params.UpdateMigratedModelRequest) error

	// Model operations
	FullModelStatus(ctx context.Context, req *params.FullModelStatusRequest) (jujuparams.FullStatus, error)
	ModelControllerInfo(ctx context.Context, model string) (*params.ModelControllerInfo, error)
	AddModelToController(ctx context.Context, req *params.AddModelToControllerRequest) (jujuparams.ModelInfo, error)
	ListModels(ctx context.Context) ([]params.ModelControllerInfoListItem, error)

	// Audit log operations
	FindAuditEvents(ctx context.Context, req *params.FindAuditEventsRequest) (params.AuditEvents, error)
	GrantAuditLogAccess(ctx context.Context, req *params.AuditLogAccessRequest) error
	RevokeAuditLogAccess(ctx context.Context, req *params.AuditLogAccessRequest) error
	PurgeLogs(ctx context.Context, req *params.PurgeLogsRequest) (*params.PurgeLogsResponse, error)

	// Role operations
	AddRole(ctx context.Context, req *params.AddRoleRequest) (params.AddRoleResponse, error)
	GetRole(ctx context.Context, req *params.GetRoleRequest) (params.GetRoleResponse, error)
	RenameRole(ctx context.Context, req *params.RenameRoleRequest) error
	RemoveRole(ctx context.Context, req *params.RemoveRoleRequest) error
	ListRoles(ctx context.Context, req *params.ListRolesRequest) ([]params.Role, error)

	// Permission operations
	AddRelation(ctx context.Context, req *params.AddRelationRequest) error
	RemoveRelation(ctx context.Context, req *params.RemoveRelationRequest) error
	CheckRelation(ctx context.Context, req *params.CheckRelationRequest) (params.CheckRelationResponse, error)
	ListRelationshipTuples(ctx context.Context, req *params.ListRelationshipTuplesRequest) (*params.ListRelationshipTuplesResponse, error)

	// Query operations
	CrossModelQuery(ctx context.Context, req *params.CrossModelQueryRequest) (*params.CrossModelQueryResponse, error)

	// Other operations
	UpgradeController(ctx context.Context, req *params.UpgradeControllerRequest) (params.UpgradeControllerResponse, error)
	UpgradeTo(ctx context.Context, req *params.UpgradeToRequest) (params.UpgradeToResponse, error)
}
