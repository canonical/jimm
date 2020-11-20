// Copyright 2020 Canonical Ltd.

package api

import (
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

// AddController adds a new controller to JIMM.
func (c *Client) AddController(req *params.AddControllerRequest) (params.ControllerInfo, error) {
	var info params.ControllerInfo
	err := c.caller.APICall("JIMM", 3, "", "AddController", req, &info)
	return info, err
}

// ListControllers returns controller info for all controllers known to
// JIMM.
func (c *Client) ListControllers() ([]params.ControllerInfo, error) {
	var resp params.ListControllersResponse
	err := c.caller.APICall("JIMM", 3, "", "ListControllers", nil, &resp)
	return resp.Controllers, err
}

// DisableControllerUUIDMasking disables UUID the masking of the real
// controller UUID with JIMM's UUID in those response.
func (c *Client) DisableControllerUUIDMasking() error {
	return c.caller.APICall("JIMM", 3, "", "DisableControllerUUIDMasking", nil, nil)
}
