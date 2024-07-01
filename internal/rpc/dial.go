// Copyright 2023 Canonical Ltd.

package rpc

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/juju/juju/core/network"
	"github.com/juju/names/v5"
	"github.com/juju/zaputil"
	"github.com/juju/zaputil/zapctx"
	"go.uber.org/zap"

	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
)

// A Dialer is used to create client connections to an RPC URL.
type Dialer struct {
	// TLSConfig is used to configure TLS for the client connection.
	TLSConfig *tls.Config
}

// Dial establishes a new client RPC connection to the given URL.
func (d Dialer) Dial(ctx context.Context, url string, headers http.Header) (*Client, error) {
	conn, err := d.DialWebsocket(ctx, url, headers)
	if err != nil {
		return nil, err
	}
	return NewClient(conn), nil
}

// DialWebsocket dials a url and returns a websocket.
func (d Dialer) DialWebsocket(ctx context.Context, url string, headers http.Header) (*websocket.Conn, error) {
	const op = errors.Op("rpc.BasicDial")

	dialer := websocket.Dialer{
		TLSClientConfig: d.TLSConfig,
	}
	conn, _, err := dialer.DialContext(context.Background(), url, headers)
	if err != nil {
		zapctx.Error(ctx, "BasicDial failed", zap.Error(err))
		return nil, errors.E(op, err)
	}
	return conn, nil
}

// Dial connects to the controller/model and returns a raw websocket
// that can be used as is.
// It accepts the endpoints to dial, normally /api or /commands.
func Dial(ctx context.Context, ctl *dbmodel.Controller, modelTag names.ModelTag, finalPath string, headers http.Header) (*websocket.Conn, error) {
	var tlsConfig *tls.Config
	if ctl.CACertificate != "" {
		cp := x509.NewCertPool()
		ok := cp.AppendCertsFromPEM([]byte(ctl.CACertificate))
		if !ok {
			zapctx.Warn(ctx, "no CA certificates added")
		}
		tlsConfig = &tls.Config{
			RootCAs:    cp,
			ServerName: ctl.TLSHostname,
		}
	}
	dialer := Dialer{
		TLSConfig: tlsConfig,
	}

	if ctl.PublicAddress != "" {
		// If there is a public-address configured it is almost
		// certainly the one we want to use, try it first.
		conn, err := dialer.DialWebsocket(ctx, websocketURL(ctl.PublicAddress, modelTag, finalPath), headers)
		if err != nil {
			zapctx.Error(ctx, "failed to dial public address", zaputil.Error(err))
		} else {
			return conn, nil
		}
	}
	var urls []string
	for _, hps := range ctl.Addresses {
		for _, hp := range hps {
			if maybeReachable(hp.Scope) {
				urls = append(urls, websocketURL(fmt.Sprintf("%s:%d", hp.Value, hp.Port), modelTag, finalPath))
			}
		}
	}
	zapctx.Debug(ctx, "Dialling all URLs", zap.Any("urls", urls))
	conn, err := dialAll(ctx, &dialer, urls, headers)
	if err != nil {
		return nil, err
	}
	return conn, nil
}

// maybeReachable decides what kinds of links JIMM should try to connect via.
// Local IPs like localhost for example are excluded but public IPs and Cloud local IPs are potentially reachable.
func maybeReachable(scope string) bool {
	switch scope {
	case string(network.ScopeCloudLocal):
		return true
	case string(network.ScopePublic):
		return true
	case "":
		return true
	default:
		return false
	}
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
func dialAll(ctx context.Context, dialer *Dialer, urls []string, headers http.Header) (*websocket.Conn, error) {
	if len(urls) == 0 {
		return nil, errors.E("no urls to dial")
	}
	conn, err := dialAllHelper(ctx, dialer, urls, headers)
	if err != nil {
		return nil, err
	}
	return conn, nil
}

// dialAllHelper simultaneously dials all given urls and returns the first successful websocket connection.
func dialAllHelper(ctx context.Context, dialer *Dialer, urls []string, headers http.Header) (*websocket.Conn, error) {
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
			conn, dErr := dialer.DialWebsocket(ctx, url, headers)
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
