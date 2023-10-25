// Copyright 2020 Canonical Ltd.

package api

import (
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
