// Copyright 2015 Canonical Ltd.

package jem

import (
	"context"
	"io"
	"net/http"
	"time"

	vault "github.com/hashicorp/vault/api"
	"gopkg.in/errgo.v1"
	"gopkg.in/macaroon-bakery.v2/bakery"
	"gopkg.in/mgo.v2"

	"github.com/CanonicalLtd/jimm/internal/debugapi"
	"github.com/CanonicalLtd/jimm/internal/jemserver"
	"github.com/CanonicalLtd/jimm/internal/jujuapi"
	"github.com/CanonicalLtd/jimm/internal/pubsub"
	v2 "github.com/CanonicalLtd/jimm/internal/v2"
	"github.com/CanonicalLtd/jimm/params"
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

	// MaxMgoSessions holds the maximum number of sessions
	// that will be held in the pool. The actual number of sessions
	// may temporarily go above this.
	MaxMgoSessions int

	// ControllerAdmin holds the identity of the user
	// or group that is allowed to create controllers.
	ControllerAdmin params.User

	// IdentityLocation holds the location of the third party identity service.
	IdentityLocation string

	// CharmstoreLocation holds the location of the charmstore
	// associated with the controller.
	CharmstoreLocation string

	// MeteringLocation holds the location of the omnibus
	// associated with the controller.
	MeteringLocation string

	// ThirdPartyLocator holds a third-party store.
	// It may be nil.
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
func NewServer(ctx context.Context, config ServerParams) (HandleCloser, error) {
	srv, err := jemserver.New(ctx, jemserver.Params(config), versions)
	if err != nil {
		return nil, errgo.Mask(err)
	}
	return srv, nil
}
