// The code in this file was automatically generated by running httprequest-generate-client.
// DO NOT EDIT

package jemclient

import (
	"github.com/CanonicalLtd/jem/params"
	"github.com/juju/httprequest"
)

type client struct {
	Client httprequest.Client
}

// AddController adds a new controller.
func (c *client) AddController(p *params.AddController) error {
	return c.Client.Call(p, nil)
}

// DeleteController removes an existing controller.
func (c *client) DeleteController(p *params.DeleteController) error {
	return c.Client.Call(p, nil)
}

// DeleteModel deletes an model from JEM.
func (c *client) DeleteModel(p *params.DeleteModel) error {
	return c.Client.Call(p, nil)
}

// GetAllControllerLocations returns all the available
// sets of controller location attributes, restricting
// the search by any provided location attributes.
func (c *client) GetAllControllerLocations(p *params.GetAllControllerLocations) (*params.AllControllerLocationsResponse, error) {
	var r *params.AllControllerLocationsResponse
	err := c.Client.Call(p, &r)
	return r, err
}

// GetController returns information on a controller.
func (c *client) GetController(p *params.GetController) (*params.ControllerResponse, error) {
	var r *params.ControllerResponse
	err := c.Client.Call(p, &r)
	return r, err
}

// GetControllerLocation returns a map of location attributes for a given controller.
func (c *client) GetControllerLocation(p *params.GetControllerLocation) (params.ControllerLocation, error) {
	var r params.ControllerLocation
	err := c.Client.Call(p, &r)
	return r, err
}

// GetControllerLocations returns all the available values for a given controller
// location attribute. The set of controllers is constrained by the URL query
// parameters.
func (c *client) GetControllerLocations(p *params.GetControllerLocations) (*params.ControllerLocationsResponse, error) {
	var r *params.ControllerLocationsResponse
	err := c.Client.Call(p, &r)
	return r, err
}

// GetControllerPerm returns the ACL for a given controller.
// Only the owner (arg.EntityPath.User) can read the ACL.
func (c *client) GetControllerPerm(p *params.GetControllerPerm) (params.ACL, error) {
	var r params.ACL
	err := c.Client.Call(p, &r)
	return r, err
}

// GetModel returns information on a given model.
func (c *client) GetModel(p *params.GetModel) (*params.ModelResponse, error) {
	var r *params.ModelResponse
	err := c.Client.Call(p, &r)
	return r, err
}

// GetModelPerm returns the ACL for a given model.
// Only the owner (arg.EntityPath.User) can read the ACL.
func (c *client) GetModelPerm(p *params.GetModelPerm) (params.ACL, error) {
	var r params.ACL
	err := c.Client.Call(p, &r)
	return r, err
}

// JujuStatus retrieves and returns the status of the specifed model.
func (c *client) JujuStatus(p *params.JujuStatus) (*params.JujuStatusResponse, error) {
	var r *params.JujuStatusResponse
	err := c.Client.Call(p, &r)
	return r, err
}

// ListController returns all the controllers stored in JEM.
// Currently the ProviderType field in each ControllerResponse is not
// populated.
func (c *client) ListController(p *params.ListController) (*params.ListControllerResponse, error) {
	var r *params.ListControllerResponse
	err := c.Client.Call(p, &r)
	return r, err
}

// ListModels returns all the models stored in JEM.
// Note that the models returned don't include the username or password.
// To gain access to a specific model, that model should be retrieved
// explicitly.
func (c *client) ListModels(p *params.ListModels) (*params.ListModelsResponse, error) {
	var r *params.ListModelsResponse
	err := c.Client.Call(p, &r)
	return r, err
}

// NewModel creates a new model inside an existing Controller.
func (c *client) NewModel(p *params.NewModel) (*params.ModelResponse, error) {
	var r *params.ModelResponse
	err := c.Client.Call(p, &r)
	return r, err
}

// SetControllerPerm sets the permissions on a controller entity.
// Only the owner (arg.EntityPath.User) can change the permissions
// on an an entity. The owner can always read an entity, even
// if it has empty ACL.
func (c *client) SetControllerPerm(p *params.SetControllerPerm) error {
	return c.Client.Call(p, nil)
}

// SetModelPerm sets the permissions on a controller entity.
// Only the owner (arg.EntityPath.User) can change the permissions
// on an an entity. The owner can always read an entity, even
// if it has empty ACL.
// TODO remove this.
func (c *client) SetModelPerm(p *params.SetModelPerm) error {
	return c.Client.Call(p, nil)
}

// UpdateCredential stores the provided credential under the provided,
// user, cloud and name. If there is already a credential with that name
// it is overwritten.
func (c *client) UpdateCredential(p *params.UpdateCredential) error {
	return c.Client.Call(p, nil)
}

// WhoAmI returns authentication information on the client that is
// making the WhoAmI call.
func (c *client) WhoAmI(p *params.WhoAmI) (params.WhoAmIResponse, error) {
	var r params.WhoAmIResponse
	err := c.Client.Call(p, &r)
	return r, err
}
