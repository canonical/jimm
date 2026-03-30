// Copyright 2026 Canonical.

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
	jujuhttp "github.com/juju/http/v2"
	"github.com/juju/juju/api/base"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v5"
	"github.com/juju/zaputil/zapctx"
	"go.uber.org/zap"
	"gopkg.in/httprequest.v1"

	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/jimmjwx"
	"github.com/canonical/jimm/v3/internal/openfga"
	"github.com/canonical/jimm/v3/internal/rpc"
	"github.com/canonical/jimm/v3/internal/servermon"
	jimmversion "github.com/canonical/jimm/v3/version"
)

const (
	adminUser = "admin"
)

// A ControllerCredentialsStore is a store for controller credentials.
type ControllerCredentialsStore interface {
	// GetControllerCredentials retrieves the credentials for the given controller from a vault
	// service.
	GetControllerCredentials(ctx context.Context, controllerName string) (string, string, error)
}

// A Dialer is an implementation of a jimm.Dialer that adapts a juju API
// connection to provide a jimm API.
type Dialer struct {
	ControllerCredentialsStore ControllerCredentialsStore
	JWTService                 *jimmjwx.JWTService
}

func (d *Dialer) createLoginRequest(ctx context.Context, ctl *dbmodel.Controller, modelTag names.ModelTag, user *openfga.User) (*jujuparams.LoginRequest, error) {
	// Always request superuser permissions, even when representing a non-admin user
	// This is only safe because we have already checked the user's openfga permissions in a layer above.
	permissions := make(map[string]string)
	permissions[ctl.ResourceTag().String()] = "superuser"
	if modelTag.Id() != "" {
		permissions[modelTag.String()] = string(jujuparams.ModelAdminAccess)
	}

	userTag := user.ResourceTag().String()

	jwt, err := d.JWTService.NewJWT(ctx, jimmjwx.JWTParams{
		Controller: ctl.ResourceTag().Id(),
		User:       userTag,
		Access:     permissions,
	})
	if err != nil {
		return nil, err
	}
	jwtString := base64.StdEncoding.EncodeToString(jwt)

	return &jujuparams.LoginRequest{
		AuthTag:       userTag,
		ClientVersion: jimmversion.ControllerVersion,
		Token:         jwtString,
	}, nil
}

// Dial implements jimm.Dialer.
func (d *Dialer) Dial(ctx context.Context, ctl *dbmodel.Controller, modelTag names.ModelTag, user *openfga.User) (*Connection, error) {

	conn, err := rpc.Dial(ctx, ctl, modelTag, "", nil, nil)
	if err != nil {
		return nil, err
	}
	if conn == nil {
		return nil, errors.Codef(errors.CodeConnectionFailed, "%w", err)
	}
	client := rpc.NewClient(conn)

	if user == nil {
		user = &openfga.User{Identity: &dbmodel.Identity{Name: adminUser}}
	}

	var loginRequest *jujuparams.LoginRequest
	loginRequest, err = d.createLoginRequest(ctx, ctl, modelTag, user)
	if err != nil {
		return nil, err
	}

	var res jujuparams.LoginResult
	if err := client.Call(ctx, "Admin", 3, "", "Login", loginRequest, &res); err != nil {
		client.Close()
		return nil, errors.Codef(errors.CodeConnectionFailed, "%w", err)
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

	monitorC := make(chan struct{})
	broken := new(uint32)
	go pinger(client, ct.Id(), monitorC, broken)
	return &Connection{
		ctx:                ctx,
		client:             client,
		user:               user,
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
func pinger(client *rpc.Client, controller string, doneC <-chan struct{}, broken *uint32) {
	doPing := func() bool {
		ctx, cancel := context.WithTimeout(context.Background(), pingTimeout)
		defer cancel()

		durationObserver := servermon.DurationObserver(servermon.JujuPingDurationHistogram, controller)
		defer durationObserver()

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
	facadeVersions     map[string]bool
	bestFacadeVersions map[string]int

	monitorC chan struct{}
	broken   *uint32

	dialer *Dialer
	user   *openfga.User
	ctl    *dbmodel.Controller
	mt     names.ModelTag
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

func (c *Connection) RootHTTPClient() (*httprequest.Client, error) {
	return c.HTTPClient()
}

// hasFacadeVersion returns whether the connection supports the given
// facade at the given version.
func (c *Connection) hasFacadeVersion(facade string, version int) bool {
	return c.facadeVersions[fmt.Sprintf("%s\x1f%d", facade, version)]
}

// Call makes an RPC call to the server. Call sends the request message to
// the server and waits for the response to be returned or the context to
// be canceled.
func (c *Connection) Call(ctx context.Context, facade string, version int, id, method string, args, resp interface{}) (err error) {
	labels := []string{facade, method, ""}
	if c.ctl != nil {
		labels = []string{facade, method, c.ctl.UUID}
	}
	durationObserver := servermon.DurationObserver(servermon.JujuCallDurationHistogram, labels...)
	defer durationObserver()
	defer servermon.ErrorCounter(servermon.JujuCallErrorCount, &err, labels...)

	err = c.client.Call(ctx, facade, version, id, method, args, resp)
	if err != nil {
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
	return fmt.Errorf("facade %v version %v not supported", facade, versions)
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
	return nil, errors.Codef(errors.CodeNotImplemented, "not implemented")
}

// BakeryClientWrapper wraps an httpbakery.Client to implement
// the MacaroonDischarger interface.
type BakeryClientWrapper struct {
	*httpbakery.Client
}

// CookieJar returns an http.CookieJar used to store macaroon cookies.
func (b BakeryClientWrapper) CookieJar() http.CookieJar {
	return b.Jar
}

// BakeryClient returns the bakery client for this connection.
func (c *Connection) BakeryClient() base.MacaroonDischarger {
	return BakeryClientWrapper{httpbakery.NewClient()}
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

	modelTag, ok := c.ModelTag()
	if !ok {
		return nil, errors.New("no model found")
	}

	user, pass, err := c.dialer.ControllerCredentialsStore.GetControllerCredentials(c.ctx, c.ctl.Name)
	if err != nil {
		return nil, err
	}
	ok = names.IsValidUser(user)
	if !ok {
		return nil, errors.New("invalid/missing controller credentials")
	}
	requestHeader := jujuhttp.BasicAuthHeader(names.NewUserTag(user).String(), pass)
	conn, err := rpc.Dial(c.ctx, c.ctl, modelTag, path, requestHeader, attrs)
	if err != nil {
		return nil, err
	}
	return conn, nil
}

// ConnectControllerStream connects to the given HTTP websocket
// endpoint path and returns the resulting connection. The given
// values are used as URL query values when making the initial
// HTTP request. Headers passed in will be added to the HTTP
// request.
func (c *Connection) ConnectControllerStream(path string, attrs url.Values, extraHeaders http.Header) (base.Stream, error) {

	user, pass, err := c.dialer.ControllerCredentialsStore.GetControllerCredentials(c.ctx, c.ctl.Name)
	if err != nil {
		return nil, err
	}

	header := jujuhttp.BasicAuthHeader(names.NewUserTag(user).String(), pass)
	for key, vals := range extraHeaders {
		for _, val := range vals {
			header.Add(key, val)
		}
	}

	conn, err := rpc.Dial(c.ctx, c.ctl, names.ModelTag{}, path, header, attrs)
	if err != nil {
		return nil, err
	}

	return conn, nil
}
