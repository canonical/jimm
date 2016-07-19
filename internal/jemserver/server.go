// Copyright 2015 Canonical Ltd.

package jemserver

import (
	"crypto/rand"
	"fmt"
	"net/http"
	"net/url"

	"github.com/juju/httprequest"
	"github.com/juju/idmclient"
	"github.com/juju/loggo"
	"github.com/julienschmidt/httprouter"
	"github.com/prometheus/client_golang/prometheus"
	"gopkg.in/errgo.v1"
	"gopkg.in/macaroon-bakery.v1/bakery"
	"gopkg.in/macaroon-bakery.v1/httpbakery"
	"gopkg.in/macaroon-bakery.v1/httpbakery/agent"
	"gopkg.in/mgo.v2"

	"github.com/CanonicalLtd/jem/internal/jem"
	"github.com/CanonicalLtd/jem/internal/monitor"
	"github.com/CanonicalLtd/jem/params"
)

var logger = loggo.GetLogger("jem.internal.jemserver")

// NewAPIHandlerFunc is a function that returns set of httprequest
// handlers that uses the given JEM pool and server params.
type NewAPIHandlerFunc func(*jem.Pool, Params) ([]httprequest.Handler, error)

// Params holds configuration for a new API server.
// It must be kept in sync with identical definition in the
// top level jem package.
type Params struct {
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
}

// Server represents a JEM HTTP server.
type Server struct {
	router  *httprouter.Router
	pool    *jem.Pool
	monitor *monitor.Monitor
}

// New returns a new handler that handles model manager
// requests and stores its data in the given database.
// The returned handler should be closed when finished
// with.
func New(config Params, versions map[string]NewAPIHandlerFunc) (*Server, error) {
	if len(versions) == 0 {
		return nil, errgo.Newf("JEM server must serve at least one version of the API")
	}
	idmClient, err := newIdentityClient(config)
	if err != nil {
		return nil, errgo.Mask(err)
	}
	jconfig := jem.Params{
		DB: config.DB,
		BakeryParams: bakery.NewServiceParams{
			// TODO The location is attached to any macaroons that we
			// mint. Currently we don't know the location of the current
			// service. We potentially provide a way to configure this,
			// but it probably doesn't matter, as nothing currently uses
			// the macaroon location for anything.
			Location: "jem",
			Locator:  config.PublicKeyLocator,
		},
		IDMClient:        idmClient,
		ControllerAdmin:  config.ControllerAdmin,
		IdentityLocation: config.IdentityLocation,
	}
	p, err := jem.NewPool(jconfig)
	if err != nil {
		return nil, errgo.Notef(err, "cannot make store")
	}
	srv := &Server{
		router: httprouter.New(),
		pool:   p,
	}
	if config.RunMonitor {
		owner, err := monitorLeaseOwner(config.AgentUsername)
		if err != nil {
			return nil, errgo.Mask(err)
		}
		srv.monitor = monitor.New(p, owner)
	}
	srv.router.Handler("GET", "/metrics", prometheus.Handler())
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

func monitorLeaseOwner(agentName string) (string, error) {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", errgo.Notef(err, "cannot make random owner")
	}
	return fmt.Sprintf("%s-%x", agentName, buf), nil
}

func newIdentityClient(config Params) (*idmclient.Client, error) {
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

// ServeHTTP implements http.Handler.Handle.
func (srv *Server) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	header := w.Header()
	ao := "*"
	if o := req.Header.Get("Origin"); o != "" {
		ao = o
	}
	header.Set("Access-Control-Allow-Origin", ao)
	header.Set("Access-Control-Allow-Headers", "Bakery-Protocol-Version, Macaroons, X-Requested-With, Content-Type")
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
	if srv.monitor != nil {
		srv.monitor.Kill()
		if err := srv.monitor.Wait(); err != nil {
			logger.Warningf("error shutting down monitor: %v", err)
		}
	}
	srv.pool.Close()
	return nil
}

// Pool returns the JEM pool used by the server.
// It is made available for testing purposes.
func (srv *Server) Pool() *jem.Pool {
	return srv.pool
}
