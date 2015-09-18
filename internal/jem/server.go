// Copyright 2015 Canonical Ltd.

package jem

import (
	"net/http"
	"net/url"

	"github.com/CanonicalLtd/blues-identity/idmclient"
	"github.com/juju/httprequest"
	"github.com/juju/loggo"
	"github.com/julienschmidt/httprouter"
	"gopkg.in/errgo.v1"
	"gopkg.in/macaroon-bakery.v1/bakery"
	"gopkg.in/macaroon-bakery.v1/httpbakery"
	"gopkg.in/macaroon-bakery.v1/httpbakery/agent"
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

	// IdentityLocation holds the location of the third party identity service.
	IdentityLocation string

	// PublicKeyLocator holds a public key store.
	// It may be nil.
	PublicKeyLocator bakery.PublicKeyLocator

	// AgentUsername and AgentKey hold the credentials used for agent
	// authentication.
	AgentUsername string
	AgentKey      *bakery.KeyPair
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
	idmClient, err := newIdentityClient(config)
	if err != nil {
		return nil, errgo.Mask(err)
	}
	p, err := NewPool(config.DB, bparams, idmClient)
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
			l, _, _ := srv.router.Lookup("OPTIONS", h.Path)
			if l == nil {
				srv.router.OPTIONS(h.Path, srv.options)
			}
		}
	}
	return srv, nil
}

func newIdentityClient(config ServerParams) (*idmclient.Client, error) {
	// Note: no need for persistent cookies as we'll
	// be able to recreate the macaroons on startup.
	bclient := httpbakery.NewClient()
	bclient.Key = config.AgentKey
	idmURL, err := url.Parse(config.IdentityLocation)
	if err != nil {
		return nil, errgo.Notef(err, "cannot parse identity location URL %q", config.IdentityLocation)
	}
	agent.SetUpAuth(bclient, idmURL, config.AgentUsername)
	return idmclient.New(idmclient.NewParams{
		BaseURL: config.IdentityLocation,
		Client:  bclient,
	}), nil
}

type Server struct {
	router *httprouter.Router
	pool   *Pool
}

// ServeHTTP implements http.Handler.Handle.
func (srv *Server) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	header := w.Header()
	ao := "*"
	if o := req.Header.Get("Origin"); o != "" {
		ao = o
	}
	header.Set("Access-Control-Allow-Origin", ao)
	header.Set("Access-Control-Allow-Headers", "Bakery-Protocol-Version, Macaroons, X-Requested-With")
	header.Set("Access-Control-Allow-Credentials", "true")
	header.Set("Access-Control-Cache-Max-Age", "600")
	// TODO: in handlers, look up methods for this request path and return only those methods here.
	header.Set("Access-Control-Allow-Methods", "DELETE,GET,HEAD,PUT,POST,OPTIONS")
	header.Set("Access-Control-Expose-Headers", "WWW-Authenticate")
	srv.router.ServeHTTP(w, req)
}

func (srv *Server) options(http.ResponseWriter, *http.Request, httprouter.Params) {
	// We don't need to do anything here because all the headers
	// required by OPTIONS are added for every request anyway.
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
