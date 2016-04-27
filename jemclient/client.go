// Copyright 2015 Canonical Ltd

// Package jemclient holds an automatically generated API interface
// to the JEM server.
package jemclient

import (
	"net/url"

	"github.com/juju/httprequest"
	"gopkg.in/macaroon-bakery.v1/httpbakery"

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
	Client  *httpbakery.Client
}

// New returns a new client.
func New(p NewParams) *Client {
	var c Client
	c.Client.BaseURL = p.BaseURL
	c.Client.Doer = p.Client
	c.Client.UnmarshalError = httprequest.ErrorUnmarshaler(new(params.Error))
	return &c
}

// GetControllerLocations returns all the available values for a given controller
// location attribute.
func (c *Client) GetControllerLocations(p *params.GetControllerLocations) (*params.ControllerLocationsResponse, error) {
	// We implement this method on Client because the automatically
	// generated method isn't sufficient to handle the general query parameters.
	q := make(url.Values)
	for attr, val := range p.Location {
		q.Set(attr, val)
	}
	var r *params.ControllerLocationsResponse
	// Technically the base URL could already contain query
	// parameters, in which case this would be invalid, but
	// that shouldn't be a problem in practice.
	err := c.Client.CallURL(c.Client.BaseURL+"?"+q.Encode(), p, &r)
	return r, err
}

// GetAllControllerLocations returns all the available
// sets of controller location attributes, restricting
// the search by the provided location attributes.
func (c *Client) GetAllControllerLocations(p *params.GetAllControllerLocations) (*params.AllControllerLocationsResponse, error) {
	// We implement this method on Client because the automatically
	// generated method isn't sufficient to handle the general query parameters.
	q := make(url.Values)
	for attr, val := range p.Location {
		q.Set(attr, val)
	}
	var r *params.AllControllerLocationsResponse
	// Technically the base URL could already contain query
	// parameters, in which case this would be invalid, but
	// that shouldn't be a problem in practice.
	err := c.Client.CallURL(c.Client.BaseURL+"?"+q.Encode(), p, &r)
	return r, err
}
