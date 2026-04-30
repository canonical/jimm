// Copyright 2026 Canonical.

package api

import (
	"github.com/juju/errors"
	jujucloud "github.com/juju/juju/cloud"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v5"

	"github.com/canonical/jimm/v3/pkg/api/params"
)

// An APICaller implements the interface required to make API calls.
type APICallCloser interface {
	// APICall makes a call to the API server with the given object type,
	// id, request and parameters. The response is filled in with the
	// call's result if the call is successful.
	APICall(objType string, version int, id, request string, params, response any) error
	Close() error
}

// Client is a client for the JIMM API.
type Client struct {
	caller APICallCloser
}

// NewClient creates a new API client for the JIMM API.
func NewClient(c APICallCloser) *Client {
	return &Client{caller: c}
}

func (c *Client) Close() error {
	return c.caller.Close()
}

// AddCloudToController adds the specified cloud to a specific controller in JIMM.
func (c *Client) AddCloudToController(req *params.AddCloudToControllerRequest) error {
	return c.caller.APICall("JIMM", 4, "", "AddCloudToController", req, nil)
}

// AddModelToController adds a new model to a controller in JIMM.
func (c *Client) AddModelToController(req *params.AddModelToControllerRequest) (jujuparams.ModelInfo, error) {
	var info jujuparams.ModelInfo
	err := c.caller.APICall("JIMM", 4, "", "AddModelToController", req, &info)
	return info, err
}

// AddController adds a new controller to JIMM.
func (c *Client) AddController(req *params.AddControllerRequest) (params.ControllerInfo, error) {
	var info params.ControllerInfo
	err := c.caller.APICall("JIMM", 4, "", "AddController", req, &info)
	return info, err
}

// DisableControllerUUIDMasking disables UUID the masking of the real
// controller UUID with JIMM's UUID in those response.
func (c *Client) DisableControllerUUIDMasking() error {
	return c.caller.APICall("JIMM", 4, "", "DisableControllerUUIDMasking", nil, nil)
}

// FindAuditEvents finds audit events that match the requested filters.
func (c *Client) FindAuditEvents(req *params.FindAuditEventsRequest) (params.AuditEvents, error) {
	var resp params.AuditEvents
	if err := c.caller.APICall("JIMM", 4, "", "FindAuditEvents", req, &resp); err != nil {
		return params.AuditEvents{}, err
	}
	return resp, nil
}

// GrantAuditLogAccess grants the given access to the audit log to the
// given user.
func (c *Client) GrantAuditLogAccess(req *params.AuditLogAccessRequest) error {
	return c.caller.APICall("JIMM", 4, "", "GrantAuditLogAccess", req, nil)
}

// ListControllers returns controller info for all controllers known to
// JIMM.
func (c *Client) ListControllers() ([]params.ControllerInfo, error) {
	var resp params.ListControllersResponse
	err := c.caller.APICall("JIMM", 4, "", "ListControllers", nil, &resp)
	return resp.Controllers, err
}

// RemoveCloudFromController removes the specified cloud from a specific controller.
func (c *Client) RemoveCloudFromController(req *params.RemoveCloudFromControllerRequest) error {
	return c.caller.APICall("JIMM", 4, "", "RemoveCloudFromController", req, nil)
}

// RemoveController removes a controller from the JAAS system. Only
// controllers that are unavailable can be removed, unless force is used.
// The return value contains the details of the controller that was
// removed.
func (c *Client) RemoveController(req *params.RemoveControllerRequest) (params.ControllerInfo, error) {
	var info params.ControllerInfo
	err := c.caller.APICall("JIMM", 4, "", "RemoveController", req, &info)
	return info, err
}

// RevokeAuditLogAccess revokes the given access to the audit log from the
// given user.
func (c *Client) RevokeAuditLogAccess(req *params.AuditLogAccessRequest) error {
	return c.caller.APICall("JIMM", 4, "", "RevokeAuditLogAccess", req, nil)
}

// SetControllerDeprecated sets the deprecated status of a controller.
func (c *Client) SetControllerDeprecated(req *params.SetControllerDeprecatedRequest) (params.ControllerInfo, error) {
	var info params.ControllerInfo
	err := c.caller.APICall("JIMM", 4, "", "SetControllerDeprecated", req, &info)
	return info, err
}

// SaveControllerProfile creates or replaces a saved controller profile.
func (c *Client) SaveControllerProfile(req *params.SaveControllerProfileRequest) (params.SaveControllerProfileResponse, error) {
	var response params.SaveControllerProfileResponse
	err := c.caller.APICall("JIMM", 4, "", "SaveControllerProfile", req, &response)
	return response, err
}

// GetControllerProfile retrieves a saved controller profile by name.
func (c *Client) GetControllerProfile(req *params.GetControllerProfileRequest) (params.GetControllerProfileResponse, error) {
	var response params.GetControllerProfileResponse
	err := c.caller.APICall("JIMM", 4, "", "GetControllerProfile", req, &response)
	return response, err
}

// ListControllerProfiles lists saved controller profiles, optionally filtered
// by Juju version.
func (c *Client) ListControllerProfiles(req *params.ListControllerProfilesRequest) ([]params.ControllerProfileSummary, error) {
	var response params.ListControllerProfilesResponse
	err := c.caller.APICall("JIMM", 4, "", "ListControllerProfiles", req, &response)
	return response.Profiles, err
}

// RemoveControllerProfile removes a saved controller profile by name.
func (c *Client) RemoveControllerProfile(req *params.RemoveControllerProfileRequest) error {
	return c.caller.APICall("JIMM", 4, "", "RemoveControllerProfile", req, nil)
}

// UpgradeTo initiates a controller upgrade to the specified version.
func (c *Client) UpgradeTo(req *params.UpgradeToRequest) (params.UpgradeToResponse, error) {
	var resp params.UpgradeToResponse
	err := c.caller.APICall("JIMM", 4, "", "UpgradeTo", req, &resp)
	return resp, err
}

// FullModelStatus returns the full status of the juju model.
func (c *Client) FullModelStatus(req *params.FullModelStatusRequest) (jujuparams.FullStatus, error) {
	var status jujuparams.FullStatus
	err := c.caller.APICall("JIMM", 4, "", "FullModelStatus", req, &status)
	return status, err
}

// ImportModel imports a model running on a controller.
func (c *Client) ImportModel(req *params.ImportModelRequest) error {
	return c.caller.APICall("JIMM", 4, "", "ImportModel", req, nil)
}

// UpdateMigratedModel updates which controller a model is running on
// following an external migration operation.
func (c *Client) UpdateMigratedModel(req *params.UpdateMigratedModelRequest) error {
	return c.caller.APICall("JIMM", 4, "", "UpdateMigratedModel", req, nil)
}

// Authorisation RPC commands

// User Groups
// AddGroup adds the group to JIMM.
func (c *Client) AddGroup(req *params.AddGroupRequest) (params.AddGroupResponse, error) {
	var resp params.AddGroupResponse
	err := c.caller.APICall("JIMM", 4, "", "AddGroup", req, &resp)
	return resp, err
}

// GetGroup returns the group with the given UUID or name. Only one should be provided.
func (c *Client) GetGroup(req *params.GetGroupRequest) (params.GetGroupResponse, error) {
	var resp params.GetGroupResponse
	err := c.caller.APICall("JIMM", 4, "", "GetGroup", req, &resp)
	return resp, err
}

// RenameGroup renames a group in JIMM.
func (c *Client) RenameGroup(req *params.RenameGroupRequest) error {
	return c.caller.APICall("JIMM", 4, "", "RenameGroup", req, nil)
}

// RemoveGroup removes a group in JIMM.
func (c *Client) RemoveGroup(req *params.RemoveGroupRequest) error {
	return c.caller.APICall("JIMM", 4, "", "RemoveGroup", req, nil)
}

// ListGroups lists the groups in JIMM.
func (c *Client) ListGroups(req *params.ListGroupsRequest) ([]params.Group, error) {
	var resp params.ListGroupResponse
	err := c.caller.APICall("JIMM", 4, "", "ListGroups", req, &resp)
	return resp.Groups, err
}

// User Roles

// AddRole adds the Role to JIMM.
func (c *Client) AddRole(req *params.AddRoleRequest) (params.AddRoleResponse, error) {
	var resp params.AddRoleResponse
	err := c.caller.APICall("JIMM", 4, "", "AddRole", req, &resp)
	return resp, err
}

// GetRole returns the Role with the given UUID or name. Only one should be provided.
func (c *Client) GetRole(req *params.GetRoleRequest) (params.GetRoleResponse, error) {
	var resp params.GetRoleResponse
	err := c.caller.APICall("JIMM", 4, "", "GetRole", req, &resp)
	return resp, err
}

// RenameRole renames a Role in JIMM.
func (c *Client) RenameRole(req *params.RenameRoleRequest) error {
	return c.caller.APICall("JIMM", 4, "", "RenameRole", req, nil)
}

// RemoveRole removes a Role in JIMM.
func (c *Client) RemoveRole(req *params.RemoveRoleRequest) error {
	return c.caller.APICall("JIMM", 4, "", "RemoveRole", req, nil)
}

// ListRoles lists the Roles in JIMM.
func (c *Client) ListRoles(req *params.ListRolesRequest) ([]params.Role, error) {
	var resp params.ListRoleResponse
	err := c.caller.APICall("JIMM", 4, "", "ListRoles", req, &resp)
	return resp.Roles, err
}

// Tuple management

// AddRelation adds a relational tuple in JIMM.
func (c *Client) AddRelation(req *params.AddRelationRequest) error {
	return c.caller.APICall("JIMM", 4, "", "AddRelation", req, nil)
}

// RemoveRelation removes a relational tuple in JIMM.
func (c *Client) RemoveRelation(req *params.RemoveRelationRequest) error {
	return c.caller.APICall("JIMM", 4, "", "RemoveRelation", req, nil)
}

// CheckRelation verifies that the object graph reaches the provided
// relation for a given user/group, relation and target object.
// This object could be another group, model, controller, etc.
// This command corresponds directly to:
// https://openfga.dev/api/service#/Relationship%20Queries/Check
func (c *Client) CheckRelation(req *params.CheckRelationRequest) (params.CheckRelationResponse, error) {
	var checkResp params.CheckRelationResponse
	err := c.caller.APICall("JIMM", 4, "", "CheckRelation", req, &checkResp)
	return checkResp, err
}

// CheckRelations performs an authorisation check for a list of tuples.
func (c *Client) CheckRelations(req *params.CheckRelationsRequest) (params.CheckRelationsResponse, error) {
	var checksResp params.CheckRelationsResponse
	err := c.caller.APICall("JIMM", 4, "", "CheckRelations", req, &checksResp)
	return checksResp, err
}

// ListRelationshipTuples returns a list of tuples matching the specified criteria.
func (c *Client) ListRelationshipTuples(req *params.ListRelationshipTuplesRequest) (*params.ListRelationshipTuplesResponse, error) {
	var response params.ListRelationshipTuplesResponse
	err := c.caller.APICall("JIMM", 4, "", "ListRelationshipTuples", req, &response)
	return &response, err
}

// CrossModelQuery enables users to query all of their available models and each entity within the model.
//
// The query will run against output exactly like "juju status --format json", but for each of their models.
func (c *Client) CrossModelQuery(req *params.CrossModelQueryRequest) (*params.CrossModelQueryResponse, error) {
	var response params.CrossModelQueryResponse
	err := c.caller.APICall("JIMM", 4, "", "CrossModelQuery", req, &response)
	return &response, err
}

// PurgeLogs purges logs from the database before the given date.
func (c *Client) PurgeLogs(req *params.PurgeLogsRequest) (*params.PurgeLogsResponse, error) {
	var response params.PurgeLogsResponse
	err := c.caller.APICall("JIMM", 4, "", "PurgeLogs", req, &response)
	return &response, err
}

// MigrateModel migrates a model between two controllers that are attached to JIMM.
func (c *Client) MigrateModel(req *params.MigrateModelRequest) (*jujuparams.InitiateMigrationResults, error) {
	var response jujuparams.InitiateMigrationResults
	err := c.caller.APICall("JIMM", 4, "", "MigrateModel", req, &response)
	return &response, err
}

// Version returns version info of the controller.
func (c *Client) Version() (params.VersionResponse, error) {
	var response params.VersionResponse
	err := c.caller.APICall("JIMM", 4, "", "Version", nil, &response)
	return response, err
}

// PrepareModelMigration prepares JIMM for an incoming ModelMigration.
func (c *Client) PrepareModelMigration(req *params.PrepareModelMigrationRequest) (params.PrepareModelMigrationResponse, error) {
	var response params.PrepareModelMigrationResponse
	err := c.caller.APICall("JIMM", 4, "", "PrepareModelMigration", req, &response)
	return response, err
}

// ListMigrationTargets returns the list of juju controllers that the given
// model could be migrated to.
func (c *Client) ListMigrationTargets(req *params.ListMigrationTargetsRequest) ([]params.ControllerInfo, error) {
	var response params.ListControllersResponse
	err := c.caller.APICall("JIMM", 4, "", "ListMigrationTargets", req, &response)
	return response.Controllers, err
}

// BootstrapInfo retrieves the status and logs of a
// bootstrap or destroy-controller job.
func (c *Client) BootstrapInfo(req *params.GetBootstrapInfoRequest) (params.GetBootstrapInfoResponse, error) {
	var response params.GetBootstrapInfoResponse
	err := c.caller.APICall("JIMM", 4, "", "BootstrapInfo", req, &response)
	return response, err
}

// StopBootstrap stops a bootstrap job on the JIMM server.
func (c *Client) StopBootstrap(req *params.StopBootstrapRequest) error {
	return c.caller.APICall("JIMM", 4, "", "StopBootstrap", req, nil)
}

// StartBootstrap starts a bootstrap operation on the JIMM server.
func (c *Client) StartBootstrap(req *params.BootstrapParams) (*params.StartBootstrapResponse, error) {
	var response params.StartBootstrapResponse
	err := c.caller.APICall("JIMM", 4, "", "StartBootstrap", req, &response)
	return &response, err
}

// StartDestroyController starts a destroy-controller operation on the JIMM server.
func (c *Client) StartDestroyController(req *params.DestroyControllerRequest) (*params.StartBootstrapResponse, error) {
	var response params.StartBootstrapResponse
	err := c.caller.APICall("JIMM", 4, "", "StartDestroyController", req, &response)
	return &response, err
}

// ListUserClouds lists the clouds available to the specified user.
func (c *Client) ListUserClouds(req *params.ListUserCloudsRequest) (map[names.CloudTag]jujucloud.Cloud, error) {
	var resp jujuparams.CloudsResult
	err := c.caller.APICall("JIMM", 4, "", "ListUserClouds", req, &resp)

	clouds := make(map[names.CloudTag]jujucloud.Cloud)
	for tagString, cloud := range resp.Clouds {
		tag, err := names.ParseCloudTag(tagString)
		if err != nil {
			return nil, errors.Trace(err)
		}
		clouds[tag] = cloudFromParams(tag.Id(), cloud)
	}
	return clouds, err
}

// ModelControllerInfo returns information about a model and the controller hosting it.
// The model parameter can be:
//   - A model tag (e.g., "model-2cb433a6-04eb-4ec4-9567-90426d20a004")
//   - Owner and model name (e.g., "alice@canonical.com/my-model")
func (c *Client) ModelControllerInfo(modelQualifier string) (*params.ModelControllerInfo, error) {
	req := params.ModelControllerInfoRequest{ModelQualifier: modelQualifier}
	var resp params.ModelControllerInfo
	err := c.caller.APICall("JIMM", 4, "", "ModelControllerInfo", req, &resp)
	return &resp, err
}

func cloudFromParams(cloudName string, p jujuparams.Cloud) jujucloud.Cloud {
	authTypes := make([]jujucloud.AuthType, len(p.AuthTypes))
	for i, authType := range p.AuthTypes {
		authTypes[i] = jujucloud.AuthType(authType)
	}
	regions := make([]jujucloud.Region, len(p.Regions))
	for i, region := range p.Regions {
		regions[i] = jujucloud.Region{
			Name:             region.Name,
			Endpoint:         region.Endpoint,
			IdentityEndpoint: region.IdentityEndpoint,
			StorageEndpoint:  region.StorageEndpoint,
		}
	}
	var regionConfig map[string]jujucloud.Attrs
	for r, attr := range p.RegionConfig {
		if regionConfig == nil {
			regionConfig = make(map[string]jujucloud.Attrs)
		}
		regionConfig[r] = attr
	}
	return jujucloud.Cloud{
		Name:              cloudName,
		Type:              p.Type,
		AuthTypes:         authTypes,
		Endpoint:          p.Endpoint,
		IdentityEndpoint:  p.IdentityEndpoint,
		StorageEndpoint:   p.StorageEndpoint,
		Regions:           regions,
		CACertificates:    p.CACertificates,
		SkipTLSVerify:     p.SkipTLSVerify,
		Config:            p.Config,
		RegionConfig:      regionConfig,
		IsControllerCloud: p.IsControllerCloud,
	}
}

// JobInfo retrieves information about a job with the given ID.
func (c *Client) JobInfo(req *params.JobInfoRequest) (*params.JobInfoResponse, error) {
	var resp params.JobInfoResponse
	err := c.caller.APICall("JIMM", 4, "", "JobInfo", req, &resp)
	if err != nil {
		return nil, err
	}
	return &resp, nil
}

// ListJobs returns a list of jobs based on the provided parameters.
func (c *Client) ListJobs(req *params.ListJobsRequest) (*params.ListJobsResponse, error) {
	var resp params.ListJobsResponse
	err := c.caller.APICall("JIMM", 4, "", "ListJobs", req, &resp)
	if err != nil {
		return nil, err
	}
	return &resp, nil
}
