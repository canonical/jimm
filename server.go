// Copyright 2015 Canonical Ltd.

package jem

import (
	"io"
	"net/http"

	"gopkg.in/macaroon-bakery.v1/bakery"
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

// HandleCloser represents an HTTP handler that can
// be closed to free resources associated with the
// handler. The Close method should not be called
// until all requests on the handler have completed.
type HandleCloser interface {
	http.Handler
	io.Closer
}

// NewServer returns a new handler that handles charm store requests and stores
// its data in the given database. The returned handler should
// be closed after use (first ensuring that all outstanding requests have
// completed).
func NewServer(config ServerParams) (HandleCloser, error) {
	return jem.NewServer(jem.ServerParams(config), versions)
}
