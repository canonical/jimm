// The code in this file was automatically generated by running
// 	httprequest-generate-client github.com/CanonicalLtd/jem/internal/v1 Handler client
// DO NOT EDIT

package jemclient

import (
	"github.com/CanonicalLtd/jem/params"
	"github.com/juju/httprequest"
)

type client struct {
	Client httprequest.Client
}

// AddJES adds a new state server.
func (c *client) AddJES(p *params.AddJES) error {
	return c.Client.Call(p, nil)
}

// AddTemplate adds a new template.
func (c *client) AddTemplate(p *params.AddTemplate) error {
	return c.Client.Call(p, nil)
}

// DeleteEnvironment deletes an environment from JEM.
func (c *client) DeleteEnvironment(p *params.DeleteEnvironment) error {
	return c.Client.Call(p, nil)
}

// RemoveJES removes an existing state server.
func (c *client) DeleteJES(p *params.DeleteJES) error {
	return c.Client.Call(p, nil)
}

// DeleteTemplate surpisingly deletes a template.
func (c *client) DeleteTemplate(p *params.DeleteTemplate) error {
	return c.Client.Call(p, nil)
}

// GetEnvironment returns information on a given environment.
func (c *client) GetEnvironment(p *params.GetEnvironment) (*params.EnvironmentResponse, error) {
	var r *params.EnvironmentResponse
	err := c.Client.Call(p, &r)
	return r, err
}

// GetEnvironmentPerm returns the ACL for a given environment.
// Only the owner (arg.EntityPath.User) can read the ACL.
func (c *client) GetEnvironmentPerm(p *params.GetEnvironmentPerm) (params.ACL, error) {
	var r params.ACL
	err := c.Client.Call(p, &r)
	return r, err
}

// GetJES returns information on a state server.
func (c *client) GetJES(p *params.GetJES) (*params.JESResponse, error) {
	var r *params.JESResponse
	err := c.Client.Call(p, &r)
	return r, err
}

// GetStateServerPerm returns the ACL for a given state server.
// Only the owner (arg.EntityPath.User) can read the ACL.
func (c *client) GetStateServerPerm(p *params.GetStateServerPerm) (params.ACL, error) {
	var r params.ACL
	err := c.Client.Call(p, &r)
	return r, err
}

// GetTemplate returns information on a single template.
func (c *client) GetTemplate(p *params.GetTemplate) (*params.TemplateResponse, error) {
	var r *params.TemplateResponse
	err := c.Client.Call(p, &r)
	return r, err
}

// GetTemplatePerm returns the ACL for a given template.
// Only the owner (arg.EntityPath.User) can read the ACL.
func (c *client) GetTemplatePerm(p *params.GetTemplatePerm) (params.ACL, error) {
	var r params.ACL
	err := c.Client.Call(p, &r)
	return r, err
}

// ListEnvironments returns all the environments stored in JEM.
func (c *client) ListEnvironments(p *params.ListEnvironments) (*params.ListEnvironmentsResponse, error) {
	var r *params.ListEnvironmentsResponse
	err := c.Client.Call(p, &r)
	return r, err
}

// ListJES returns all the state servers stored in JEM.
// Currently the Template  and ProviderType field in each JESResponse is not
// populated.
func (c *client) ListJES(p *params.ListJES) (*params.ListJESResponse, error) {
	var r *params.ListJESResponse
	err := c.Client.Call(p, &r)
	return r, err
}

// ListTemplates returns information on all accessible templates.
func (c *client) ListTemplates(p *params.ListTemplates) (*params.ListTemplatesResponse, error) {
	var r *params.ListTemplatesResponse
	err := c.Client.Call(p, &r)
	return r, err
}

// NewEnvironment creates a new environment inside an existing JES.
func (c *client) NewEnvironment(p *params.NewEnvironment) (*params.EnvironmentResponse, error) {
	var r *params.EnvironmentResponse
	err := c.Client.Call(p, &r)
	return r, err
}

// SetEnvironmentPerm sets the permissions on a state server entity.
// Only the owner (arg.EntityPath.User) can change the permissions
// on an an entity. The owner can always read an entity, even
// if it has empty ACL.
func (c *client) SetEnvironmentPerm(p *params.SetEnvironmentPerm) error {
	return c.Client.Call(p, nil)
}

// SetStateServerPerm sets the permissions on a state server entity.
// Only the owner (arg.EntityPath.User) can change the permissions
// on an an entity. The owner can always read an entity, even
// if it has empty ACL.
func (c *client) SetStateServerPerm(p *params.SetStateServerPerm) error {
	return c.Client.Call(p, nil)
}

// SetTemplatePerm sets the permissions on a template entity.
// Only the owner (arg.EntityPath.User) can change the permissions
// on an entity. The owner can always read an entity, even
// if it has an empty ACL.
func (c *client) SetTemplatePerm(p *params.SetTemplatePerm) error {
	return c.Client.Call(p, nil)
}
