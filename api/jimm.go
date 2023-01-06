// Copyright 2020 Canonical Ltd.

package api

import (
	jujuparams "github.com/juju/juju/rpc/params"

	"github.com/CanonicalLtd/jimm/api/params"
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
	return c.caller.APICall("JIMM", 3, "", "AddCloudToController", req, nil)
}

// AddController adds a new controller to JIMM.
func (c *Client) AddController(req *params.AddControllerRequest) (params.ControllerInfo, error) {
	var info params.ControllerInfo
	err := c.caller.APICall("JIMM", 3, "", "AddController", req, &info)
	return info, err
}

// DisableControllerUUIDMasking disables UUID the masking of the real
// controller UUID with JIMM's UUID in those response.
func (c *Client) DisableControllerUUIDMasking() error {
	return c.caller.APICall("JIMM", 3, "", "DisableControllerUUIDMasking", nil, nil)
}

// FindAuditEvents finds audit events that match the requested filters.
func (c *Client) FindAuditEvents(req *params.FindAuditEventsRequest) (params.AuditEvents, error) {
	var resp params.AuditEvents
	if err := c.caller.APICall("JIMM", 3, "", "FindAuditEvents", req, &resp); err != nil {
		return params.AuditEvents{}, err
	}
	return resp, nil
}

// GrantAuditLogAccess grants the given access to the audit log to the
// given user.
func (c *Client) GrantAuditLogAccess(req *params.AuditLogAccessRequest) error {
	return c.caller.APICall("JIMM", 3, "", "GrantAuditLogAccess", req, nil)
}

// ListControllers returns controller info for all controllers known to
// JIMM.
func (c *Client) ListControllers() ([]params.ControllerInfo, error) {
	var resp params.ListControllersResponse
	err := c.caller.APICall("JIMM", 3, "", "ListControllers", nil, &resp)
	return resp.Controllers, err
}

// AddCloudToController adds the specified cloud to a specific controller in JIMM.
func (c *Client) RemoveCloudFromController(req *params.RemoveCloudFromControllerRequest) error {
	return c.caller.APICall("JIMM", 3, "", "RemoveCloudFromController", req, nil)
}

// RemoveController removes a controller from the JAAS system. Only
// controllers that are unavailable can be removed, unless force is used.
// The return value contains the details of the controller that was
// removed.
func (c *Client) RemoveController(req *params.RemoveControllerRequest) (params.ControllerInfo, error) {
	var info params.ControllerInfo
	err := c.caller.APICall("JIMM", 3, "", "RemoveController", req, &info)
	return info, err
}

// RevokeAuditLogAccess revokes the given access to the audit log from the
// given user.
func (c *Client) RevokeAuditLogAccess(req *params.AuditLogAccessRequest) error {
	return c.caller.APICall("JIMM", 3, "", "RevokeAuditLogAccess", req, nil)
}

// SetControllerDeprecated sets the deprecated status of a controller.
func (c *Client) SetControllerDeprecated(req *params.SetControllerDeprecatedRequest) (params.ControllerInfo, error) {
	var info params.ControllerInfo
	err := c.caller.APICall("JIMM", 3, "", "SetControllerDeprecated", req, &info)
	return info, err
}

// FullModelStatus returns the full status of the juju model.
func (c *Client) FullModelStatus(req *params.FullModelStatusRequest) (jujuparams.FullStatus, error) {
	var status jujuparams.FullStatus
	err := c.caller.APICall("JIMM", 3, "", "FullModelStatus", req, &status)
	return status, err
}

// ImportModel imports a model running on a controller.
func (c *Client) ImportModel(req *params.ImportModelRequest) error {
	return c.caller.APICall("JIMM", 3, "", "ImportModel", req, nil)
}

// UpdateMigratedModel updates which controller a model is running on
// following an external migration operation.
func (c *Client) UpdateMigratedModel(req *params.UpdateMigratedModelRequest) error {
	return c.caller.APICall("JIMM", 3, "", "UpdateMigratedModel", req, nil)
}

// AddGroup adds the group to JIMM.
func (c *Client) AddGroup(req *params.AddGroupRequest) error {
	return c.caller.APICall("JIMM", 4, "", "AddGroup", req, nil)
}

// RenameGroup renames a group.
func (c *Client) RenameGroup(req *params.RenameGroupRequest) error {
	return c.caller.APICall("JIMM", 4, "", "RenameGroup", req, nil)
}
