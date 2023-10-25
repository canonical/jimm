// Copyright 2015 Canonical Ltd

// Package jemclient holds an automatically generated API interface
// to the JEM server.
package jemclient

import (
	"gopkg.in/httprequest.v1"

	"github.com/canonical/jimm/params"
)

//go:generate httprequest-generate-client github.com/canonical/jimm/internal/v2 Handler client

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
