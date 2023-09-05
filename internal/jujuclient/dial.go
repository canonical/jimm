// Copyright 2020 Canonical Ltd.

// Package jujuclient is the client JIMM uses to connect to juju
// controllers. The jujuclient uses the juju RPC API directly using
// API-native types, mostly those coming from github.com/juju/names and
// github.com/juju/juju/apiserver/params. The rationale for this being that
// as JIMM both sends and receives messages accross this API it should
// perform as little format conversion as possible.
package jujuclient

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"fmt"
	"net/url"
	"path"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v4"
	"github.com/juju/zaputil"
	"github.com/juju/zaputil/zapctx"
	"go.uber.org/zap"

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

// A ControllerCredentialsStore is a store for controller credentials.
type ControllerCredentialsStore interface {
	// GetControllerCredentials retrieves the credentials for the given controller from a vault
	// service.
	GetControllerCredentials(ctx context.Context, controllerName string) (string, string, error)

	// PutControllerCredentials stores the controller credentials in a vault
	// service.
	PutControllerCredentials(ctx context.Context, controllerName string, username string, password string) error
}

// A Dialer is an implementation of a jimm.Dialer that adapts a juju API
// connection to provide a jimm API.
type Dialer struct {
	JWTService *jimmjwx.JWTService
}

// Dial implements jimm.Dialer.
func (d *Dialer) Dial(ctx context.Context, ctl *dbmodel.Controller, modelTag names.ModelTag, requiredPermissions map[string]string) (jimm.API, error) {
	const op = errors.Op("jujuclient.Dial")

	var tlsConfig *tls.Config
	if ctl.CACertificate != "" {
		cp := x509.NewCertPool()
		ok := cp.AppendCertsFromPEM([]byte(ctl.CACertificate))
		if !ok {
			zapctx.Warn(ctx, "no CA certificates added")
		}
		tlsConfig = &tls.Config{
			RootCAs: cp,
		}
	}
	dialer := rpc.Dialer{
		TLSConfig: tlsConfig,
	}

	var client *rpc.Client
	var err error
	if ctl.PublicAddress != "" {
		// If there is a public-address configured it is almost
		// certainly the one we want to use, try it first.
		client, err = dialer.Dial(ctx, websocketURL(ctl.PublicAddress, modelTag, ""))
		if err != nil {
			zapctx.Error(ctx, "failed to dial public address", zaputil.Error(err))
		}
	}
	if client == nil {
		var urls []string
		for _, hps := range ctl.Addresses {
			for _, hp := range hps {
				if hp.Scope != "public" && hp.Scope != "" {
					continue
				}
				u := websocketURL(fmt.Sprintf("%s:%d", hp.Value, hp.Port), modelTag, "")
				urls = append(urls, u)
			}
		}
		var err2 error
		client, err2 = dialAll(ctx, &dialer, urls)
		if err == nil {
			err = err2
		}
	}
	if client == nil {
		return nil, errors.E(op, errors.CodeConnectionFailed, err)
	}

	// JIMM is automatically given all required permissions
	permissions := requiredPermissions
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
		return nil, errors.E(op, err)
	}
	jwtString := base64.StdEncoding.EncodeToString(jwt)

	args := jujuparams.LoginRequest{
		AuthTag:       names.NewUserTag("admin").String(),
		ClientVersion: jujuClientVersion,
		Token:         jwtString,
	}

	var res jujuparams.LoginResult
	if err := client.Call(ctx, "Admin", 3, "", "Login", args, &res); err != nil {
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
	for _, fv := range res.Facades {
		for _, v := range fv.Versions {
			facades[fmt.Sprintf("%s\x1f%d", fv.Name, v)] = true
		}
	}

	monitorC := make(chan struct{})
	broken := new(uint32)
	go monitor(client, monitorC, broken)
	return &Connection{
		client:         client,
		userTag:        args.AuthTag,
		facadeVersions: facades,
		monitorC:       monitorC,
		broken:         broken,
		dialer:         d,
		ctl:            ctl,
		mt:             modelTag,
	}, nil
}

// ProxyDial is similar to the other dial methods but returns a raw websocket
// that can be used as is.
// Whereas Dial always dials the /api endpont, ProxyDial accepts the endpoints to dial,
// normally /api or /commands.
func ProxyDial(ctx context.Context, ctl *dbmodel.Controller, modelTag names.ModelTag, finalPath string) (*websocket.Conn, error) {
	const op = errors.Op("jujuclient.ProxyDial")

	var tlsConfig *tls.Config
	if ctl.CACertificate != "" {
		cp := x509.NewCertPool()
		ok := cp.AppendCertsFromPEM([]byte(ctl.CACertificate))
		if !ok {
			zapctx.Warn(ctx, "no CA certificates added")
		}
		tlsConfig = &tls.Config{
			RootCAs: cp,
		}
	}
	dialer := rpc.Dialer{
		TLSConfig: tlsConfig,
	}

	if ctl.PublicAddress != "" {
		// If there is a public-address configured it is almost
		// certainly the one we want to use, try it first.
		conn, err := dialer.DialWebsocket(ctx, websocketURL(ctl.PublicAddress, modelTag, finalPath))
		if err != nil {
			zapctx.Error(ctx, "failed to dial public address", zaputil.Error(err))
		} else {
			return conn, nil
		}
	}
	var urls []string
	for _, hps := range ctl.Addresses {
		for _, hp := range hps {
			if hp.Scope != "public" && hp.Scope != "" {
				continue
			}
			urls = append(urls, websocketURL(fmt.Sprintf("%s:%d", hp.Value, hp.Port), modelTag, finalPath))
		}
	}
	zapctx.Debug(ctx, "Dialling all URLs", zap.Any("urls", urls))
	conn, err := dialAllwebsocket(ctx, &dialer, urls)
	if err != nil {
		return nil, err
	}
	return conn, nil
}

func websocketURL(s string, mt names.ModelTag, finalPath string) string {
	u := url.URL{
		Scheme: "wss",
		Host:   s,
	}
	if mt.Id() != "" {
		u.Path = path.Join(u.Path, "model", mt.Id())
	}
	if finalPath == "" {
		u.Path = path.Join(u.Path, "api")
	} else {
		u.Path = path.Join(u.Path, finalPath)
	}
	return u.String()
}

// dialAll simultaneously dials all given urls and returns the first
// connection.
func dialAll(ctx context.Context, dialer *rpc.Dialer, urls []string) (*rpc.Client, error) {
	if len(urls) == 0 {
		return nil, errors.E("no urls to dial")
	}
	conn, err := dialAllHelper(ctx, dialer, urls)
	if err != nil {
		return nil, err
	}
	return rpc.NewClient(conn), nil
}

// dialAllwebsocket is similar to dialAll but returns a raw websocket connection instead of a client.
func dialAllwebsocket(ctx context.Context, dialer *rpc.Dialer, urls []string) (*websocket.Conn, error) {
	if len(urls) == 0 {
		return nil, errors.E("no urls to dial")
	}
	conn, err := dialAllHelper(ctx, dialer, urls)
	if err != nil {
		return nil, err
	}
	return conn, nil
}

// dialAllHelper simultaneously dials all given urls and returns the first successful websocket connection.
func dialAllHelper(ctx context.Context, dialer *rpc.Dialer, urls []string) (*websocket.Conn, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var clientOnce, errOnce sync.Once
	var err error
	var wg sync.WaitGroup
	var res *websocket.Conn
	for _, url := range urls {
		zapctx.Info(ctx, "dialing", zap.String("url", url))
		url := url
		wg.Add(1)
		go func() {
			defer wg.Done()
			conn, dErr := dialer.DialWebsocket(ctx, url)
			if dErr != nil {
				errOnce.Do(func() {
					err = dErr
				})
				return
			}
			var keep bool
			clientOnce.Do(func() {
				res = conn
				keep = true
				cancel()
			})
			if !keep {
				conn.Close()
			}
		}()
	}
	wg.Wait()
	if res != nil {
		return res, nil
	}
	return nil, err
}

const pingTimeout = 15 * time.Second
const pingInterval = 30 * time.Second

// monitor runs in the background ensuring the client connection is kept alive.
func monitor(client *rpc.Client, doneC <-chan struct{}, broken *uint32) {
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
// earliest facade versions that support all the required data, but will
// fall-back to earlier versions with slightly degraded functionality if
// possible.
type Connection struct {
	client         *rpc.Client
	userTag        string
	facadeVersions map[string]bool

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
