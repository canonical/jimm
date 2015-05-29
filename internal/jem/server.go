// Copyright 2015 Canonical Ltd.

package jem

import (
	"net/http"

	"github.com/juju/httprequest"
	"github.com/juju/loggo"
	"github.com/julienschmidt/httprouter"
	"gopkg.in/errgo.v1"
	"gopkg.in/macaroon-bakery.v1/bakery"
	"gopkg.in/mgo.v2"
)

var logger = loggo.GetLogger("jem.internal.jem")

// NewAPIHandlerFunc is a function that returns set of httprequest
// handlers that uses the given JEM pool and server params.
type NewAPIHandlerFunc func(*Pool, ServerParams) ([]httprequest.Handler, error)

// ServerParams holds configuration for a new API server.
// It must be kept in sync with identical definition in the
// top level jem package.
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

// NewServer returns a new handler that handles environment manager
// requests and stores its data in the given database.
// The returned handler should be closed when finished
// with.
func NewServer(config ServerParams, versions map[string]NewAPIHandlerFunc) (*Server, error) {
	if len(versions) == 0 {
		return nil, errgo.Newf("JEM server must serve at least one version of the API")
	}
	bparams := bakery.NewServiceParams{
		// TODO The location is attached to any macaroons that we
		// mint. Currently we don't know the location of the current
		// service. We potentially provide a way to configure this,
		// but it probably doesn't matter, as nothing currently uses
		// the macaroon location for anything.
		Location: "jem",
		Locator:  config.PublicKeyLocator,
	}
	p, err := NewPool(config.DB, &bparams)
	if err != nil {
		return nil, errgo.Notef(err, "cannot make store")
	}
	srv := &Server{
		router: httprouter.New(),
		pool:   p,
	}
	for name, newAPI := range versions {
		handlers, err := newAPI(p, config)
		if err != nil {
			return nil, errgo.Notef(err, "cannot create API %s", name)
		}
		for _, h := range handlers {
			srv.router.Handle(h.Method, h.Path, h.Handle)
		}
	}
	return srv, nil
}

type Server struct {
	router *httprouter.Router
	pool   *Pool
}

// ServeHTTP implements http.Handler.Handle.
func (srv *Server) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	srv.router.ServeHTTP(w, req)
}

// Close implements io.Closer.Close. It should not be called
// until all requests on the handler have completed.
func (srv *Server) Close() error {
	srv.pool.Close()
	return nil
}

// Pool returns the JEM pool used by the server.
// It is made available for testing purposes.
func (srv *Server) Pool() *Pool {
	return srv.pool
}
