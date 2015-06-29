// Copyright 2015 Canonical Ltd

// Package jemclient holds an automatically generated API interface
// to the JEM server.
package jemclient

import (
	"github.com/juju/httprequest"
	"gopkg.in/macaroon-bakery.v1/httpbakery"

	"github.com/CanonicalLtd/jem/params"
)

//go:generate httprequest-generate-client github.com/CanonicalLtd/jem/internal/v1 Handler client

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
