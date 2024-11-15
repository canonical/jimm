// Copyright 2024 Canonical.
package jujuclient

import (
	"context"
	"encoding/base64"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/docker/docker/client"
	"github.com/juju/juju/api"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/client/applicationoffers"
	"github.com/juju/juju/api/client/cloud"
	"github.com/juju/juju/api/client/modelmanager"
	"github.com/juju/juju/api/client/storage"
	"github.com/juju/juju/api/connector"
	"github.com/juju/juju/api/controller/controller"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v5"
	"golang.org/x/sync/singleflight"

	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/jimmjwx"
)

type JWTLoginProvider struct {
	Permissions   map[string]string
	ModelTag      names.ModelTag
	ControllerTag names.ControllerTag
	JWTService    *jimmjwx.JWTService
}

func (jlp *JWTLoginProvider) createLoginRequest(ctx context.Context) (*jujuparams.LoginRequest, error) {
	// JIMM is automatically given all required permissions
	permissions := jlp.Permissions
	if permissions == nil {
		permissions = make(map[string]string)
	}
	permissions[jlp.ControllerTag.String()] = "superuser"
	if jlp.ModelTag.Id() != "" {
		permissions[jlp.ModelTag.String()] = "admin"
	}

	jwt, err := jlp.JWTService.NewJWT(ctx, jimmjwx.JWTParams{
		Controller: jlp.ControllerTag.Id(),
		User:       names.NewUserTag("admin").String(),
		Access:     permissions,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to build jwt: %w", err)
	}
	jwtString := base64.StdEncoding.EncodeToString(jwt)

	return &jujuparams.LoginRequest{
		AuthTag:       names.NewUserTag("admin").String(),
		ClientVersion: jujuClientVersion,
		Token:         jwtString,
	}, nil
}

// Implements juju/api.LoginProvider.Login.
//
// Login performs log in when connecting to the controller.
func (jlp *JWTLoginProvider) Login(ctx context.Context, caller base.APICaller) (*api.LoginResultParams, error) {
	loginReq, err := jlp.createLoginRequest(ctx)
	if err != nil {
		return nil, err
	}

	var loginRes jujuparams.LoginResult
	if err := caller.APICall("Admin", 3, "", "Login", loginReq, &loginRes); err != nil {
		return nil, err
	}

	loginResParams, err := api.NewLoginResultParams(loginRes)
	if err != nil {
		return nil, err
	}

	return loginResParams, nil
}

// Implements juju/api.LoginProvider.AuthHeader.
//
// AuthHeader returns an HTTP header used for authentication.
// This is normally used as part of basic authentication in scenarios where a client
// makes use of a StreamConnector like when fetching logs using `juju debug-log`.
// Can return [ErrorLoginFirst] when the provider requires an RPC login before basic auth
// can be performed.
// Other errors are also possible indicating an internal error in the provider.
func (jlp *JWTLoginProvider) AuthHeader() (http.Header, error) {
	return nil, nil
}

type JujuDialer struct {
	JWTService *jimmjwx.JWTService
}

// TODO(ale8k): Don't pass a dbmodel.Controller, create params for this.
func (jd *JujuDialer) Dial(ctx context.Context, ctl *dbmodel.Controller, modelTag names.ModelTag, requiredPermissions map[string]string) (api.Connection, error) {
	cfg := connector.SimpleConfig{}
	if ctl.PublicAddress != "" {
		cfg.ControllerAddresses = []string{ctl.PublicAddress}
	} else {
		for _, hps := range ctl.Addresses {
			for _, hp := range hps {
				cfg.ControllerAddresses = append(cfg.ControllerAddresses, net.JoinHostPort(hp.Value, strconv.Itoa(hp.Port)))
			}
		}
	}

	if ctl.CACertificate != "" {
		cfg.CACert = ctl.CACertificate
	}
	if modelTag.Id() != "" {
		cfg.ModelUUID = modelTag.Id()
	}
	// TODO(ale8k):
	// Required for now as the simple config checks for user/clientid
	// Perhaps have a separate validation method that is run after creation
	// of SimpleConnector.
	cfg.Username = "placeholder"

	loginProvider := &JWTLoginProvider{
		JWTService:    jd.JWTService,
		ControllerTag: ctl.ResourceTag(),
		ModelTag:      modelTag,
		Permissions:   requiredPermissions,
	}
	connector, err := connector.NewSimple(
		cfg,
		api.WithLoginProvider(loginProvider),
	)
	if err != nil {
		return nil, err
	}

	connection, err := connector.Connect()
	if err != nil {
		return nil, err
	}

	return connection, nil
}

// cacheJujuDialer caches connections and if one is broken, attempts to re-establish it.
type cacheJujuDialer struct {
	// dialer holds the JujuDialer.
	dialer JujuDialer

	// mu is for protecting the map when retieving/adding two different
	// cached controller connections. We could in theory just have separate maps per
	// controller, but this works fine.
	mu sync.Mutex
	// sf to handle duplicates when attempting to insert a connection for the same controller.
	sf singleflight.Group

	// conns stores the connections.
	conns map[string]api.Connection

	// cleanupIntervalTicker cleans up broken connections in the cache.
	cleanupIntervalTicker *time.Ticker
}

func NewCacheDialer(d JujuDialer, cleanupInterval time.Duration) *cacheJujuDialer {
	cjd := &cacheJujuDialer{
		dialer:                d,
		cleanupIntervalTicker: time.NewTicker(cleanupInterval),
		conns:                 make(map[string]api.Connection),
	}
	go cjd.cleanupConnections()
	return cjd
}

// Dial works like so:
// - Use singleflight for:
//   - Preventing duplicate cache inserts
//   - Allow only one routine to access the cache at a time
//
// - Use a map of connections for:
//   - Preventing the need to connect multiple times to the same controller
//
// - Mux to protect the map (there are libs for this though, perhaps use one of those)
func (cjd *cacheJujuDialer) Dial(ctx context.Context, ctl *dbmodel.Controller, modelTag names.ModelTag, requiredPermissions map[string]string) (api.Connection, error) {
	if modelTag.Id() != "" {
		return cjd.dialer.Dial(ctx, ctl, modelTag, requiredPermissions)
	}

	ctlUuid := ctl.ResourceTag().Id()

	v, err, _ := cjd.sf.Do(ctlUuid, func() (interface{}, error) {
		cjd.mu.Lock()
		conn, exists := cjd.conns[ctlUuid]
		cjd.mu.Unlock()

		if exists && !conn.IsBroken() {
			return conn, nil
		}

		conn, err := cjd.dialer.Dial(ctx, ctl, modelTag, requiredPermissions)
		if err != nil {
			return nil, err
		}

		cjd.mu.Lock()
		cjd.conns[ctlUuid] = conn
		cjd.mu.Unlock()

		return conn, nil
	})

	if err != nil {
		return nil, err
	}

	return v.(api.Connection), nil
}

// cleanupConnections checks for broken connections and:
//
// - Removes them from the cache map
// - Closes them
// - Forgets the controller from the singleflight
func (cd *cacheJujuDialer) cleanupConnections() {
	for range cd.cleanupIntervalTicker.C {
		cd.mu.Lock()
		for key, conn := range cd.conns {
			if conn.IsBroken() {
				conn.Close() // TODO(ale8k): Do we need this?
				cd.sf.Forget(conn.ControllerTag().Id())
				delete(cd.conns, key)
			}
		}
		cd.mu.Unlock()
	}
}

type jujuClient struct {
	dialer *JujuDialer

	modelManager      *modelmanager.Client
	controller        *controller.Client
	cloud             *cloud.Client
	applicationOffers *applicationoffers.Client
	storage           *storage.Client
	client            *client.Client
}

// TODO(ale8k): Find a nice way to force the DialParams to specify controller or models only.
func NewJujuClient(jwtService *jimmjwx.JWTService) (*jujuClient, error) {
	jc := &jujuClient{
		dialer: &JujuDialer{
			JWTService: jwtService,
		},
	}

	return jc, nil
}
