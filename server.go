// Copyright 2015 Canonical Ltd.

package jem

import (
	"net/http"

	"gopkg.in/macaroon-bakery.v0/bakery"
	"gopkg.in/mgo.v2"

	"github.com/CanonicalLtd/jem/internal/jem"
	"github.com/CanonicalLtd/jem/internal/v1"
)

var versions = map[string]jem.NewAPIHandlerFunc{
	"v1": v1.NewAPIHandler,
}

// ServerParams holds configuration for a new API server.
type ServerParams struct {
	// DB holds the mongo database that will be used to
	// store the JEM information.
	DB *mgo.Database

	// StateServerAdmin holds the identity of the user
	// or group that is allowed to create state servers.
	StateServerAdmin string

	// IdentityLocation holds the location of the third party authorization
	// service to use when creating third party caveats.
	IdentityLocation string

	// PublicKeyLocator holds a public key store.
	// It may be nil.
	PublicKeyLocator bakery.PublicKeyLocator
}

// NewServer returns a new handler that handles charm store requests and stores
// its data in the given database.
func NewServer(config ServerParams) (http.Handler, error) {
	return jem.NewServer(jem.ServerParams(config), versions)
}
