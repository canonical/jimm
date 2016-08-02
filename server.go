// Copyright 2015 Canonical Ltd.

package jem

import (
	"io"
	"net/http"

	"gopkg.in/errgo.v1"
	"gopkg.in/macaroon-bakery.v1/bakery"
	"gopkg.in/mgo.v2"

	"github.com/CanonicalLtd/jem/internal/debugapi"
	"github.com/CanonicalLtd/jem/internal/jemserver"
	"github.com/CanonicalLtd/jem/internal/jujuapi"
	"github.com/CanonicalLtd/jem/internal/v2"
	"github.com/CanonicalLtd/jem/params"
)

var versions = map[string]jemserver.NewAPIHandlerFunc{
	"v2":    v2.NewAPIHandler,
	"debug": debugapi.NewAPIHandler,
	"juju":  jujuapi.NewAPIHandler,
}

// ServerParams holds configuration for a new API server.
type ServerParams struct {
	// DB holds the mongo database that will be used to
	// store the JEM information.
	DB *mgo.Database

	// ControllerAdmin holds the identity of the user
	// or group that is allowed to create controllers.
	ControllerAdmin params.User

	// IdentityLocation holds the location of the third party identity service.
	IdentityLocation string

	// PublicKeyLocator holds a public key store.
	// It may be nil.
	PublicKeyLocator bakery.PublicKeyLocator

	// AgentUsername and AgentKey hold the credentials used for agent
	// authentication.
	AgentUsername string
	AgentKey      *bakery.KeyPair

	// RunMonitor specifies that the monitor worker should be run.
	// This should always be set when running the server in production.
	RunMonitor bool

	// ControllerUUID holds the UUID the JIMM controller uses to
	// identify itself.
	ControllerUUID string

	// DefaultCloud is the name of the cloud to use when it is not
	// specified by the client.
	DefaultCloud string
}

// HandleCloser represents an HTTP handler that can
// be closed to free resources associated with the
// handler. The Close method should not be called
// until all requests on the handler have completed.
type HandleCloser interface {
	http.Handler
	io.Closer
}

// NewServer returns a new handler that handles JEM requests and stores
// its data in the given database. The returned handler should
// be closed after use (first ensuring that all outstanding requests have
// completed).
func NewServer(config ServerParams) (HandleCloser, error) {
	srv, err := jemserver.New(jemserver.Params(config), versions)
	if err != nil {
		return nil, errgo.Mask(err)
	}
	return srv, nil
}
