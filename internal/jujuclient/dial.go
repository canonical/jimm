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
	"fmt"
	"net/url"
	"path"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v4"
	"github.com/juju/zaputil"
	"github.com/juju/zaputil/zapctx"
	"go.uber.org/zap"

	"github.com/CanonicalLtd/jimm/internal/dbmodel"
	"github.com/CanonicalLtd/jimm/internal/errors"
	"github.com/CanonicalLtd/jimm/internal/jimm"
	"github.com/CanonicalLtd/jimm/internal/rpc"
)

const (
	// JIMM claims to be a 2.9.33 client.
	jujuClientVersion = "2.9.33"
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
	ControllerCredentialsStore ControllerCredentialsStore
}

// Dial implements jimm.Dialer.
func (d *Dialer) Dial(ctx context.Context, ctl *dbmodel.Controller, modelTag names.ModelTag) (jimm.API, error) {
	const op = errors.Op("jujuclient.Dial")

	var tlsConfig *tls.Config
	if ctl.CACertificate != "" {
		cp := x509.NewCertPool()
		cp.AppendCertsFromPEM([]byte(ctl.CACertificate))
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
		client, err = dialer.Dial(ctx, websocketURL(ctl.PublicAddress, modelTag))
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
				urls = append(urls, websocketURL(fmt.Sprintf("%s:%d", hp.Value, hp.Port), modelTag))
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

	username := ctl.AdminUser
	password := ctl.AdminPassword
	if d.ControllerCredentialsStore != nil {
		u, p, err := d.ControllerCredentialsStore.GetControllerCredentials(ctx, ctl.Name)
		if err != nil {
			return nil, errors.E(op, errors.CodeNotFound)
		}
		if u != "" {
			username = u
		}
		if password != "" {
			password = p
		}
	}

	args := jujuparams.LoginRequest{
		AuthTag:       names.NewUserTag(username).String(),
		Credentials:   password,
		ClientVersion: jujuClientVersion,
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
	}, nil
}

func websocketURL(s string, mt names.ModelTag) string {
	u := url.URL{
		Scheme: "wss",
		Host:   s,
	}
	if mt.Id() != "" {
		u.Path = path.Join(u.Path, "model", mt.Id())
	}
	u.Path = path.Join(u.Path, "api")
	return u.String()
}

// dialAll simultaneously dials all given urls and returns the first
// connection.
func dialAll(ctx context.Context, dialer *rpc.Dialer, urls []string) (*rpc.Client, error) {
	if len(urls) == 0 {
		return nil, errors.E("no urls to dial")
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var clientOnce, errOnce sync.Once
	var client *rpc.Client
	var err error
	var wg sync.WaitGroup

	for _, url := range urls {
		zapctx.Info(ctx, "dialing", zap.String("url", url))
		url := url
		wg.Add(1)
		go func() {
			defer wg.Done()
			cl, dErr := dialer.Dial(ctx, url)
			if dErr != nil {
				errOnce.Do(func() {
					err = dErr
				})
				return
			}
			var keep bool
			clientOnce.Do(func() {
				client = cl
				keep = true
				cancel()
			})
			if !keep {
				cl.Close()
			}
		}()
	}
	wg.Wait()
	if client == nil {
		return nil, err
	}
	return client, nil
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
}

// Close closes the connection.
func (c Connection) Close() error {
	close(c.monitorC)
	return c.client.Close()
}

// IsBroken returns true if the connection has failed.
func (c Connection) IsBroken() bool {
	if atomic.LoadUint32(c.broken) != 0 {
		return true
	}
	return c.client.IsBroken()
}

// hasFacadeVersion returns whether the connection supports the given
// facade at the given version.
func (c Connection) hasFacadeVersion(facade string, version int) bool {
	return c.facadeVersions[fmt.Sprintf("%s\x1f%d", facade, version)]
}

// CallHighestFacadeVersion calls the specified method on the highest supported version of
// the facade.
func (c Connection) CallHighestFacadeVersion(ctx context.Context, facade string, versions []int, id, method string, args, resp interface{}) error {
	sort.Sort(sort.Reverse(sort.IntSlice(versions)))
	for _, version := range versions {
		if c.hasFacadeVersion(facade, version) {
			return c.client.Call(ctx, facade, version, id, method, args, resp)
		}
	}
	return errors.E(fmt.Sprintf("facade %v version %v not supported", facade, versions))
}
