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
	"sync"
	"sync/atomic"
	"time"

	jujuparams "github.com/juju/juju/apiserver/params"
	"github.com/juju/names/v4"
	"go.uber.org/zap"

	"github.com/CanonicalLtd/jimm/internal/dbmodel"
	"github.com/CanonicalLtd/jimm/internal/errors"
	"github.com/CanonicalLtd/jimm/internal/jimm"
	"github.com/CanonicalLtd/jimm/internal/rpc"
	"github.com/CanonicalLtd/jimm/internal/zapctx"
)

// A Dialer is an implementation of a jimm.Dialer that adapts a juju API
// connection to provide a jimm API.
type Dialer struct{}

// Dial implements jimm.Dialer.
func (Dialer) Dial(ctx context.Context, ctl *dbmodel.Controller, modelTag names.ModelTag) (jimm.API, error) {
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

	args := jujuparams.LoginRequest{
		AuthTag:       names.NewUserTag(ctl.AdminUser).String(),
		Credentials:   ctl.AdminPassword,
		ClientVersion: "2.9.0", // claim to be a 2.9 client.
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

const pingTimeout = 30 * time.Second
const pingInterval = time.Minute

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
