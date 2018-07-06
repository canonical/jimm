// Copyright 2015 Canonical Ltd

// Package jemclient holds an automatically generated API interface
// to the JEM server.
package jemclient

import (
	"net/url"

	"github.com/juju/httprequest"

	"github.com/CanonicalLtd/jem/params"
)

//go:generate httprequest-generate-client github.com/CanonicalLtd/jem/internal/v2 Handler client

// Client represents the client of a JEM server.
type Client struct {
	client
}

// NewParams holds the parameters for creating
// a new client.
type NewParams struct {
	BaseURL string
	Client  httprequest.Doer
}

// New returns a new client.
func New(p NewParams) *Client {
	var c Client
	c.Client.BaseURL = p.BaseURL
	c.Client.Doer = p.Client
	c.Client.UnmarshalError = httprequest.ErrorUnmarshaler(new(params.Error))
	return &c
}

// The methods below are implemented on Client because the automatically
// generated method isn't sufficient to handle the general query parameters.

// GetControllerLocations returns all the available values for a given controller
// location attribute.
func (c *Client) GetControllerLocations(p *params.GetControllerLocations) (*params.ControllerLocationsResponse, error) {
	var r *params.ControllerLocationsResponse
	err := c.callWithLocationAttrs(p.Location, p, &r)
	return r, err
}

// GetAllControllerLocations returns all the available
// sets of controller location attributes, restricting
// the search by the provided location attributes.
func (c *Client) GetAllControllerLocations(p *params.GetAllControllerLocations) (*params.AllControllerLocationsResponse, error) {
	var r *params.AllControllerLocationsResponse
	err := c.callWithLocationAttrs(p.Location, p, &r)
	return r, err
}

// callWithLocationAttrs makes an API call to the endpoint implied
// by p, attaching the given location attributes as URL query parameters,
// and storing the result into the value pointed to by the value in r.
func (c *Client) callWithLocationAttrs(location map[string]string, p, r interface{}) error {
	q := make(url.Values)
	for attr, val := range location {
		q.Set(attr, val)
	}
	// Technically the base URL could already contain query
	// parameters, in which case this would be invalid, but
	// that shouldn't be a problem in practice.
	return c.Client.CallURL(c.Client.BaseURL+"?"+q.Encode(), p, r)
}
