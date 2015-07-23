// The code in this file was automatically generated by running
// 	httprequest-generate-client github.com/CanonicalLtd/jem/internal/v1 Handler client
// DO NOT EDIT

package jemclient

import (
	"github.com/juju/httprequest"

	"github.com/CanonicalLtd/jem/params"
)

type client struct {
	Client httprequest.Client
}

// AddJES adds a new state server.
func (c *client) AddJES(p *params.AddJES) error {
	return c.Client.Call(p, nil)
}

// GetEnvironment returns information on a given environment.
func (c *client) GetEnvironment(p *params.GetEnvironment) (*params.EnvironmentResponse, error) {
	var r *params.EnvironmentResponse
	err := c.Client.Call(p, &r)
	return r, err
}

// GetJES returns information on a state server.
func (c *client) GetJES(p *params.GetJES) (*params.JESResponse, error) {
	var r *params.JESResponse
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

// NewEnvironment creates a new environment inside an existing JES.
func (c *client) NewEnvironment(p *params.NewEnvironment) (*params.EnvironmentResponse, error) {
	var r *params.EnvironmentResponse
	err := c.Client.Call(p, &r)
	return r, err
}
