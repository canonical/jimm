// Copyright 2020 Canonical Ltd.

package api

import (
	jujuparams "github.com/juju/juju/rpc/params"

	"github.com/canonical/jimm/api/params"
)

// An APICaller implements the interface required to make API calls.
type APICaller interface {
	// APICall makes a call to the API server with the given object type,
	// id, request and parameters. The response is filled in with the
	// call's result if the call is successful.
	APICall(objType string, version int, id, request string, params, response interface{}) error
}

// Client is a client for the JIMM API.
type Client struct {
	caller APICaller
}

// NewClient creates a new API client for the JIMM API.
func NewClient(c APICaller) *Client {
	return &Client{caller: c}
}

// AddCloudToController adds the specified cloud to a specific controller in JIMM.
func (c *Client) AddCloudToController(req *params.AddCloudToControllerRequest) error {
	return c.caller.APICall("JIMM", 4, "", "AddCloudToController", req, nil)
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
func (c *Client) AddGroup(req *params.AddGroupRequest) error {
	return c.caller.APICall("JIMM", 4, "", "AddGroup", req, nil)
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
func (c *Client) ListGroups() ([]params.Group, error) {
	var resp params.ListGroupResponse
	err := c.caller.APICall("JIMM", 4, "", "ListGroups", nil, &resp)
	return resp.Groups, err
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
