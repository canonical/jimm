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

// CallerTokenMinter mints a caller JWT for a user operating on a
// specific controller and set of resource tags (models, application
// offers, etc.). It is satisfied by *jujuauth.Factory.
type CallerTokenMinter interface {
	NewCallerLoginToken(ctx context.Context, resourceTags []names.Tag, ctl *dbmodel.Controller, user *openfga.User) ([]byte, error)
}

// A Dialer is an implementation of a jimm.Dialer that adapts a juju API
// connection to provide a jimm API.
type Dialer struct {
	// TokenMinter mints caller-scoped JWT tokens for on-behalf-of-user dials.
	TokenMinter CallerTokenMinter
	// JWTService signs superuser JWT tokens for JIMM service-identity dials.
	JWTService    *jimmjwx.JWTService
	AdminUsername string
}

// NewDialer creates a new Dialer from dependencies.
func NewDialer(jwtService *jimmjwx.JWTService, tokenMinter CallerTokenMinter, controllerUUID string) *Dialer {
	return &Dialer{
		JWTService:  jwtService,
		TokenMinter: tokenMinter,
		// The admin username is a Juju external user, just like the JIMM users.
		AdminUsername: fmt.Sprintf("jaas-%s@external", controllerUUID),
	}
}

// newServiceJWTToken mints a superuser JWT for JIMM's own service-identity
// operations (watcher, upgrade, housekeeping). The token is issued under the
// JIMM admin username, not a real user.
func (d *Dialer) newServiceJWTToken(ctx context.Context, ctl *dbmodel.Controller, modelTag names.ModelTag) (string, error) {
	permissions := map[string]string{
		ctl.ResourceTag().String(): "superuser",
	}
	if modelTag.Id() != "" {
		permissions[modelTag.String()] = string(jujuparams.ModelAdminAccess)
	}
	jwt, err := d.JWTService.NewJWT(ctx, jimmjwx.JWTParams{
		Controller: ctl.ResourceTag().Id(),
		User:       names.NewUserTag(d.AdminUsername).String(),
		Access:     permissions,
	})
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(jwt), nil
}

// createLoginRequest creates a jujuparams.LoginRequest for the given
// controller, resource tags and user.
func (d *Dialer) createLoginRequest(ctx context.Context, ctl *dbmodel.Controller, resourceTags []names.Tag, user *openfga.User) (*jujuparams.LoginRequest, error) {
	jwt, err := d.TokenMinter.NewCallerLoginToken(ctx, resourceTags, ctl, user)
	if err != nil {
		return nil, err
	}
	return &jujuparams.LoginRequest{
		AuthTag:       user.ResourceTag().String(),
		ClientVersion: jimmversion.ControllerVersion,
		Token:         base64.StdEncoding.EncodeToString(jwt),
	}, nil
}

// DialModel implements jimm.Dialer. It creates a model-scoped connection on
// behalf of a real user. The user must be non-nil; use DialModelAsService for
// JIMM's own internal operations that have no associated user.
func (d *Dialer) DialModel(ctx context.Context, user *openfga.User, ctl *dbmodel.Controller, modelTag names.ModelTag) (*Connection, error) {
	if user == nil {
		return nil, errors.New("DialModel requires a non-nil user")
	}
	loginRequest, err := d.createLoginRequest(ctx, ctl, []names.Tag{modelTag}, user)
	if err != nil {
		return nil, err
	}
	return d.dial(ctx, ctl, modelTag, user, loginRequest, false)
}

// DialController implements jimm.Dialer. It creates a controller-scoped
// connection (so controller-level facades are available) whose JWT carries
// the user's access claims for the given resource tags (models,
// application offers, etc.).
func (d *Dialer) DialController(ctx context.Context, user *openfga.User, ctl *dbmodel.Controller, resourceTags ...names.Tag) (*Connection, error) {
	if user == nil {
		return nil, errors.New("DialController requires a non-nil user")
	}
	loginRequest, err := d.createLoginRequest(ctx, ctl, resourceTags, user)
	if err != nil {
		return nil, err
	}
	return d.dial(ctx, ctl, names.ModelTag{}, user, loginRequest, false)
}

// dial establishes a connection and logs in with the given login request.
// The connModelTag scopes the connection (empty means controller-scoped);
// the user is recorded on the connection for later token minting (e.g.
// authorization headers). asService marks whether the connection uses
// JIMM's own service identity rather than a real user.
func (d *Dialer) dial(ctx context.Context, ctl *dbmodel.Controller, connModelTag names.ModelTag, user *openfga.User, loginRequest *jujuparams.LoginRequest, asService bool) (*Connection, error) {
	conn, err := rpc.Dial(ctx, ctl, connModelTag, "", nil, nil)
	if err != nil {
		return nil, err
	}
	if conn == nil {
		return nil, errors.Codef(errors.CodeConnectionFailed, "no connection established")
	}

	client := rpc.NewClient(conn)

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
		mt:                 connModelTag,
		asService:          asService,
	}, nil
}

// DialModelAsService dials the given model on behalf of JIMM itself (no
// user); see the Dialer interface godoc.
func (d *Dialer) DialModelAsService(ctx context.Context, ctl *dbmodel.Controller, modelTag names.ModelTag) (*Connection, error) {
	return d.dialAsService(ctx, ctl, modelTag)
}

// DialControllerAsService dials the given controller on behalf of JIMM
// itself (no user), using a controller-scoped connection; see the Dialer
// interface godoc.
func (d *Dialer) DialControllerAsService(ctx context.Context, ctl *dbmodel.Controller) (*Connection, error) {
	return d.dialAsService(ctx, ctl, names.ModelTag{})
}

// dialAsService dials the given controller/model on behalf of JIMM itself
// (no user), minting a superuser token under the JIMM service identity.
func (d *Dialer) dialAsService(ctx context.Context, ctl *dbmodel.Controller, modelTag names.ModelTag) (*Connection, error) {
	jwtString, err := d.newServiceJWTToken(ctx, ctl, modelTag)
	if err != nil {
		return nil, err
	}
	loginRequest := &jujuparams.LoginRequest{
		AuthTag:       names.NewUserTag(d.AdminUsername).String(),
		ClientVersion: jimmversion.ControllerVersion,
		Token:         jwtString,
	}
	serviceUser := &openfga.User{Identity: &dbmodel.Identity{Name: d.AdminUsername}}
	return d.dial(ctx, ctl, modelTag, serviceUser, loginRequest, true)
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
	// asService is true when the connection was established under JIMM's
	// own service identity (via dialAsService), as opposed to on behalf of
	// a real user. It determines which token type is minted for
	// authorization headers.
	asService bool
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
func (c *Connection) Call(ctx context.Context, facade string, version int, id, method string, args, resp any) (err error) {
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
func (c *Connection) CallHighestFacadeVersion(ctx context.Context, facade string, versions []int, id, method string, args, resp any) error {
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
func (c *Connection) APICall(objType string, version int, id, request string, params, response any) error {
	return c.Call(c.ctx, objType, version, id, request, params, response)
}

// Context returns the standard context for this connection.
func (c *Connection) Context() context.Context {
	return c.ctx
}

func (c *Connection) authorizationHeader(modelTag names.ModelTag, extraHeaders http.Header) (http.Header, error) {
	var jwtString string
	var err error
	if c.asService {
		// Service connection: mint a superuser token under the admin identity.
		jwtString, err = c.dialer.newServiceJWTToken(c.ctx, c.ctl, modelTag)
	} else {
		// User connection: mint a caller-scoped token.
		var resourceTags []names.Tag
		if modelTag.Id() != "" {
			resourceTags = []names.Tag{modelTag}
		}
		var jwt []byte
		jwt, err = c.dialer.TokenMinter.NewCallerLoginToken(c.ctx, resourceTags, c.ctl, c.user)
		jwtString = base64.StdEncoding.EncodeToString(jwt)
	}
	if err != nil {
		return nil, err
	}
	header := make(http.Header)
	header.Set("Authorization", "Bearer "+jwtString)
	for key, vals := range extraHeaders {
		for _, val := range vals {
			header.Add(key, val)
		}
	}
	return header, nil
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

	requestHeader, err := c.authorizationHeader(modelTag, nil)
	if err != nil {
		return nil, err
	}
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
	header, err := c.authorizationHeader(names.ModelTag{}, extraHeaders)
	if err != nil {
		return nil, err
	}

	conn, err := rpc.Dial(c.ctx, c.ctl, names.ModelTag{}, path, header, attrs)
	if err != nil {
		return nil, err
	}

	return conn, nil
}
