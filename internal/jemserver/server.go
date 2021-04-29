// Copyright 2015 Canonical Ltd.

package jemserver

import (
	"context"
	"net/http"
	"time"

	vault "github.com/hashicorp/vault/api"
	"github.com/julienschmidt/httprouter"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"gopkg.in/errgo.v1"
	"gopkg.in/httprequest.v1"
	"gopkg.in/macaroon-bakery.v2/bakery"

	"github.com/CanonicalLtd/jimm/internal/dashboard"
	"github.com/CanonicalLtd/jimm/internal/pubsub"
	"github.com/CanonicalLtd/jimm/params"
)

// NewAPIHandlerFunc is a function that returns set of httprequest
// handlers that uses the given JEM pool and server params.
type NewAPIHandlerFunc func(context.Context, HandlerParams) ([]httprequest.Handler, error)

// Params holds configuration for a new API server.
// It must be kept in sync with identical definition in the
// top level jem package.
type Params struct {
	// ControllerAdmin holds the identity of the user
	// or group that is allowed to create controllers.
	ControllerAdmin params.User

	// IdentityLocation holds the location of the third party identity service.
	IdentityLocation string

	// CharmstoreLocation holds the location of the charmstore
	// associated with the controller.
	CharmstoreLocation string

	// MeteringLocation holds the location of the metering service
	// associated with the controller.
	MeteringLocation string

	// ThirdPartyLocator holds a third-party info store. It may be
	// nil.
	ThirdPartyLocator bakery.ThirdPartyLocator

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

	// WebsocketRequestTimeout is the time to wait before failing a
	// connection because the server has not received a request.
	WebsocketRequestTimeout time.Duration

	// GUILocation holds the address that serves the GUI that will be
	// used with this controller.
	GUILocation string

	// Domain holds the domain to which users must belong, not
	// including the leading "@". If this is empty, users may be in
	// any domain.
	Domain string

	// PublicCloudMetadata contains the path of the file containing
	// the public cloud metadata. If this is empty or the file
	// doesn't exist the default public cloud information is used.
	PublicCloudMetadata string

	// JujuDashboardLocation contains the path to the folder
	// where the Juju Dashboard tarball was extracted.
	JujuDashboardLocation string

	Pubsub *pubsub.Hub

	// VaultClient is the (optional) vault client to use to store
	// cloud credentials.
	VaultClient *vault.Client

	// VaultPath is the root path in the vault for JIMM's secrets.
	VaultPath string
}

// HandlerParams are the parameters used to initialize a handler.
type HandlerParams struct {
	Params
}

// Server represents a JEM HTTP server.
type Server struct {
	router  *httprouter.Router
	context context.Context
}

// New returns a new handler that handles model manager
// requests and stores its data in the given database.
// The returned handler should be closed when finished
// with.
func New(ctx context.Context, config Params, versions map[string]NewAPIHandlerFunc) (*Server, error) {
	if len(versions) == 0 {
		return nil, errgo.Newf("JEM server must serve at least one version of the API")
	}
	router := httprouter.New()

	if config.JujuDashboardLocation != "" {
		err := dashboard.Register(ctx, router, config.JujuDashboardLocation)
		if err != nil {
			return nil, errgo.Mask(err)
		}
	}

	srv := &Server{
		router:  router,
		context: ctx,
	}
	srv.router.Handler("GET", "/metrics", promhttp.Handler())
	for name, newAPI := range versions {
		handlers, err := newAPI(ctx, HandlerParams{
			Params: config,
		})
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
	return nil
}
