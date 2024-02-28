// Copyright 2020 Canonical Ltd.

// Package jujuclient is the client JIMM uses to connect to juju
// controllers. The jujuclient uses the juju RPC API directly using
// API-native types, mostly those coming from github.com/juju/names and
// github.com/juju/juju/apiserver/params. The rationale for this being that
// as JIMM both sends and receives messages across this API it should
// perform as little format conversion as possible.
package jujuclient

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"sync/atomic"
	"time"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/httpbakery"
	"github.com/juju/juju/api/base"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v5"
	"github.com/juju/zaputil/zapctx"
	"go.uber.org/zap"
	"gopkg.in/httprequest.v1"

	"github.com/canonical/jimm/internal/dbmodel"
	"github.com/canonical/jimm/internal/errors"
	"github.com/canonical/jimm/internal/jimm"
	"github.com/canonical/jimm/internal/jimmjwx"
	"github.com/canonical/jimm/internal/rpc"
)

const (
	// JIMM claims to be a 3.2.4 client.
	jujuClientVersion = "3.2.4"
)

// A Dialer is an implementation of a jimm.Dialer that adapts a juju API
// connection to provide a jimm API.
type Dialer struct {
	JWTService *jimmjwx.JWTService
}

func (d *Dialer) createLoginRequest(ctx context.Context, ctl *dbmodel.Controller, modelTag names.ModelTag, p map[string]string) (*jujuparams.LoginRequest, error) {
	// JIMM is automatically given all required permissions
	permissions := p
	if permissions == nil {
		permissions = make(map[string]string)
	}
	permissions[ctl.ResourceTag().String()] = "superuser"
	if modelTag.Id() != "" {
		permissions[modelTag.String()] = "admin"
	}

	jwt, err := d.JWTService.NewJWT(ctx, jimmjwx.JWTParams{
		Controller: ctl.UUID,
		User:       names.NewUserTag("admin").String(),
		Access:     permissions,
	})
	if err != nil {
		return nil, errors.E(err)
	}
	jwtString := base64.StdEncoding.EncodeToString(jwt)

	return &jujuparams.LoginRequest{
		AuthTag:       names.NewUserTag("admin").String(),
		ClientVersion: jujuClientVersion,
		Token:         jwtString,
	}, nil
}

// Dial implements jimm.Dialer.
func (d *Dialer) Dial(ctx context.Context, ctl *dbmodel.Controller, modelTag names.ModelTag, requiredPermissions map[string]string) (jimm.API, error) {
	const op = errors.Op("jujuclient.Dial")

	conn, err := rpc.Dial(ctx, ctl, modelTag, "")
	if err != nil {
		return nil, err
	}
	if conn == nil {
		return nil, errors.E(op, errors.CodeConnectionFailed, err)
	}
	client := rpc.NewClient(conn)

	loginRequest, err := d.createLoginRequest(ctx, ctl, modelTag, requiredPermissions)
	if err != nil {
		return nil, errors.E(op, err)
	}

	var res jujuparams.LoginResult
	if err := client.Call(ctx, "Admin", 3, "", "Login", loginRequest, &res); err != nil {
		client.Close()
		return nil, errors.E(op, errors.CodeConnectionFailed, "authentication failed", err)
	}

	ct, err := names.ParseControllerTag(res.ControllerTag)
	if err == nil {
		ctl.SetTag(ct)
	}
	if res.ServerVersion != "" {
		ctl.AgentVersion = res.ServerVersion
	}
	ctl.Addresses = dbmodel.HostPorts(res.Servers)
	facades := make(map[string]bool)
	bestFacadeVersions := make(map[string]int)
	for _, fv := range res.Facades {
		sort.Sort(sort.Reverse(sort.IntSlice(fv.Versions)))
		bestFacadeVersions[fv.Name] = fv.Versions[0]
		for _, v := range fv.Versions {
			facades[fmt.Sprintf("%s\x1f%d", fv.Name, v)] = true
		}
	}
	zapctx.Error(ctx, "facades", zap.Any("version", bestFacadeVersions))

	monitorC := make(chan struct{})
	broken := new(uint32)
	go pinger(client, monitorC, broken)
	return &Connection{
		ctx:                ctx,
		client:             client,
		userTag:            loginRequest.AuthTag,
		facadeVersions:     facades,
		bestFacadeVersions: bestFacadeVersions,
		monitorC:           monitorC,
		broken:             broken,
		dialer:             d,
		ctl:                ctl,
		mt:                 modelTag,
	}, nil
}

const pingTimeout = 15 * time.Second
const pingInterval = 30 * time.Second

// pinger runs in the background ensuring the client connection is kept alive.
func pinger(client *rpc.Client, doneC <-chan struct{}, broken *uint32) {
	doPing := func() bool {
		ctx, cancel := context.WithTimeout(context.Background(), pingTimeout)
		defer cancel()
		if err := client.Call(ctx, "Pinger", 1, "", "Ping", nil, nil); err != nil {
			zapctx.Error(ctx, "connection failed", zap.Error(err))
			return false
		}
		return true
	}

	t := time.NewTimer(pingInterval)
	defer t.Stop()
	for {
		select {
		case <-doneC:
			atomic.StoreUint32(broken, 1)
			return
		case <-t.C:
			if !doPing() {
				atomic.StoreUint32(broken, 1)
				return
			}
		}
	}
}

// A Connection is a connection to a juju controller. Connection methods
// are generally thin wrappers around juju RPC calls, although there are
// some more JIMM specific operations. The RPC calls prefer to use the
// most recent facade versions that support all the required data, but will
// fall-back to earlier versions with slightly degraded functionality if
// possible.
type Connection struct {
	ctx                context.Context
	client             *rpc.Client
	userTag            string
	facadeVersions     map[string]bool
	bestFacadeVersions map[string]int

	monitorC chan struct{}
	broken   *uint32

	dialer      *Dialer
	redialCount atomic.Int32
	ctl         *dbmodel.Controller
	mt          names.ModelTag
}

// Close closes the connection.
func (c *Connection) Close() error {
	close(c.monitorC)
	return c.client.Close()
}

// IsBroken returns true if the connection has failed.
func (c *Connection) IsBroken() bool {
	if atomic.LoadUint32(c.broken) != 0 {
		return true
	}
	return c.client.IsBroken()
}

// hasFacadeVersion returns whether the connection supports the given
// facade at the given version.
func (c *Connection) hasFacadeVersion(facade string, version int) bool {
	return c.facadeVersions[fmt.Sprintf("%s\x1f%d", facade, version)]
}

func (c *Connection) redial(ctx context.Context, requiredPermissions map[string]string) error {
	const op = errors.Op("jujuclient.redial")
	dialCount := c.redialCount.Add(1)
	if dialCount > 10 {
		return errors.E(op, "dial count exceeded")
	}
	api, err := c.dialer.Dial(ctx, c.ctl, c.mt, requiredPermissions)
	if err != nil {
		return errors.E(op, err)
	}
	if err = c.Close(); err != nil {
		return errors.E(op, err)
	}
	conn := api.(*Connection)
	c.client = conn.client
	c.userTag = conn.userTag
	c.facadeVersions = conn.facadeVersions
	c.monitorC = conn.monitorC
	c.broken = conn.broken
	return nil
}

// Call makes an RPC call to the server. Call sends the request message to
// the server and waits for the response to be returned or the context to
// be canceled.
func (c *Connection) Call(ctx context.Context, facade string, version int, id, method string, args, resp interface{}) error {
	err := c.client.Call(ctx, facade, version, id, method, args, resp)
	if err != nil {
		if rpcErr, ok := err.(*rpc.Error); ok {
			// if we get a permission check required error, we redial the controller
			// and amend permissions to include any required permissions as
			// JIMM should be allowed to access anything in the JIMM system.
			if rpcErr.Code == rpc.PermissionCheckRequiredErrorCode {
				requiredPermissions := make(map[string]string)
				for k, v := range rpcErr.Info {
					vString, ok := v.(string)
					if !ok {
						return errors.E(fmt.Sprintf("expected %T, received %T", vString, v))
					}
					requiredPermissions[k] = vString
				}
				if err = c.redial(ctx, requiredPermissions); err != nil {
					return err
				}

				return c.Call(ctx, facade, version, id, method, args, resp)
			}
		}
		return err
	}
	return nil
}

// CallHighestFacadeVersion calls the specified method on the highest supported version of
// the facade.
func (c *Connection) CallHighestFacadeVersion(ctx context.Context, facade string, versions []int, id, method string, args, resp interface{}) error {
	sort.Sort(sort.Reverse(sort.IntSlice(versions)))

	for _, version := range versions {
		if c.hasFacadeVersion(facade, version) {
			return c.Call(ctx, facade, version, id, method, args, resp)
		}
	}
	return errors.E(fmt.Sprintf("facade %v version %v not supported", facade, versions))
}

// BestFacadeVersion returns the newest version of 'objType' that this
// client can use with the current API server.
func (c *Connection) BestFacadeVersion(facade string) int {
	return c.bestFacadeVersions[facade]
}

// ModelTag returns the tag of the model the client is connected
// to if there is one. It returns false for a controller-only connection.
func (c *Connection) ModelTag() (names.ModelTag, bool) {
	return c.mt, c.mt.Id() != ""
}

// HTTPClient returns a httprequest.Client that can be used
// to make HTTP requests to the API. URLs passed to the client
// will be made relative to the API host and the current model.
func (c *Connection) HTTPClient() (*httprequest.Client, error) {
	return nil, errors.E(errors.CodeNotImplemented)
}

// BakeryClient returns the bakery client for this connection.
func (c *Connection) BakeryClient() base.MacaroonDischarger {
	return httpbakery.NewClient()
}

// APICall makes a call to the API server with the given object type,
// id, request and parameters. The response is filled in with the
// call's result if the call is successful.
func (c *Connection) APICall(objType string, version int, id, request string, params, response interface{}) error {
	return c.Call(c.ctx, objType, version, id, request, params, response)
}

// Context returns the standard context for this connection.
func (c *Connection) Context() context.Context {
	return c.ctx
}

// ConnectStream connects to the given HTTP websocket
// endpoint path (interpreted relative to the receiver's
// model) and returns the resulting connection.
// The given parameters are used as URL query values
// when making the initial HTTP request.
func (c *Connection) ConnectStream(path string, attrs url.Values) (base.Stream, error) {
	return nil, errors.E(errors.CodeNotImplemented)
}

// ConnectControllerStream connects to the given HTTP websocket
// endpoint path and returns the resulting connection. The given
// values are used as URL query values when making the initial
// HTTP request. Headers passed in will be added to the HTTP
// request.
func (c *Connection) ConnectControllerStream(path string, attrs url.Values, headers http.Header) (base.Stream, error) {
	return nil, errors.E(errors.CodeNotImplemented)
}
