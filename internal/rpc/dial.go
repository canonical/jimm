// Copyright 2026 Canonical.

package rpc

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/gorilla/websocket"
	"github.com/juju/juju/core/network"
	"github.com/juju/names/v5"
	"github.com/juju/zaputil/zapctx"
	"go.uber.org/zap"

	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
)

// A TrackedConn is a websocket connection that removes itself from
// the package registry on Close. All connections returned by Dial
// are of this type.
type TrackedConn struct {
	*websocket.Conn
}

// Close closes the connection and removes it from the registry.
func (c *TrackedConn) Close() error {
	untrackConn(c.Conn)
	return c.Conn.Close()
}

// ConnInfo describes a controller connection opened via Dial.
type ConnInfo struct {
	Controller string
	ModelTag   string
	DialStack  string
}

var (
	// connTracking enables recording controller connections opened
	// via Dial. Test setups only.
	connTracking atomic.Bool

	regMu    sync.Mutex
	registry = map[*websocket.Conn]ConnInfo{}
)

// EnableConnTracking enables recording controller connections opened
// via Dial, so tests can detect leaked connections. Test setups only.
func EnableConnTracking() {
	connTracking.Store(true)
}

func trackConn(conn *websocket.Conn, info ConnInfo) {
	if !connTracking.Load() {
		return
	}
	info.DialStack = captureDialStack()
	regMu.Lock()
	defer regMu.Unlock()
	registry[conn] = info
}

func untrackConn(conn *websocket.Conn) {
	if !connTracking.Load() {
		return
	}
	regMu.Lock()
	defer regMu.Unlock()
	delete(registry, conn)
}

// ActiveControllerConnections returns the controller connections
// opened via Dial that have not been closed yet.
func ActiveControllerConnections() map[*websocket.Conn]ConnInfo {
	regMu.Lock()
	defer regMu.Unlock()
	m := make(map[*websocket.Conn]ConnInfo, len(registry))
	for conn, info := range registry {
		m[conn] = info
	}
	return m
}

// ResetConnTracking clears the connection registry. Test cleanup use
// only: it drops references to any leaked connections (and their
// stack traces) so they don't affect subsequent tests.
func ResetConnTracking() {
	regMu.Lock()
	defer regMu.Unlock()
	clear(registry)
}

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
	return NewClient(&TrackedConn{Conn: conn}), nil
}

// DialWebsocket dials a url and returns a websocket.
func (d Dialer) DialWebsocket(ctx context.Context, url string, headers http.Header) (*websocket.Conn, error) {

	dialer := websocket.Dialer{
		TLSClientConfig: d.TLSConfig,
	}
	conn, resp, err := dialer.DialContext(ctx, url, headers)
	if err != nil {
		return nil, fmt.Errorf("basic dial failed: %w", err)
	}
	defer resp.Body.Close()
	return conn, nil
}

// GetAddressesAndTLSConfig returns the addresses and TLS configuration for the given controller.
func GetAddressesAndTLSConfig(ctx context.Context, ctl *dbmodel.Controller) ([]string, *tls.Config) {
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
			MinVersion: tls.VersionTLS12,
		}
	}
	var addrs []string
	if ctl.PublicAddress != "" {
		addrs = append(addrs, ctl.PublicAddress)
	}

	for _, hps := range ctl.Addresses {
		for _, hp := range hps {
			if maybeReachable(network.Scope(hp.Scope)) {
				var ip string
				if hp.Type == string(network.IPv6Address) {
					ip = fmt.Sprintf("[%s]:%d", hp.Value, hp.Port)
				} else {
					ip = fmt.Sprintf("%s:%d", hp.Value, hp.Port)
				}
				addrs = append(addrs, ip)
			}
		}
	}
	return addrs, tlsConfig
}

// Dial connects to the controller/model and returns a websocket
// that can be used as is. It accepts the endpoints to dial,
// normally /api or /commands.
func Dial(ctx context.Context, ctl *dbmodel.Controller, modelTag names.ModelTag, finalPath string, headers http.Header, attrs url.Values) (*TrackedConn, error) {
	addrs, tlsConfig := GetAddressesAndTLSConfig(ctx, ctl)
	dialer := Dialer{
		TLSConfig: tlsConfig,
	}
	var websocketUrls []string
	for _, addr := range addrs {
		websocketUrls = append(websocketUrls, websocketURL(addr, modelTag, finalPath, attrs))
	}
	zapctx.Debug(ctx, "Dialling all URLs", zap.Any("urls", websocketUrls))
	conn, err := dialAll(ctx, &dialer, websocketUrls, headers)
	if err != nil {
		return nil, err
	}
	trackConn(conn, ConnInfo{
		Controller: ctl.Name,
		ModelTag:   modelTag.Id(),
	})
	return &TrackedConn{Conn: conn}, nil
}

// captureDialStack returns a compact stack trace identifying the
// caller of Dial, skipping frames inside this package.
func captureDialStack() string {
	pcs := make([]uintptr, 32)
	n := runtime.Callers(2, pcs)
	frames := runtime.CallersFrames(pcs[:n])
	var sb strings.Builder
	for {
		frame, more := frames.Next()
		if !strings.Contains(frame.File, "internal/rpc/") {
			fmt.Fprintf(&sb, "%s\n  %s:%d\n", frame.Function, frame.File, frame.Line)
		}
		if !more {
			break
		}
	}
	return sb.String()
}

// maybeReachable decides what kinds of links JIMM should try to connect via.
// Local IPs like localhost for example are excluded but public IPs and Cloud local IPs are potentially reachable.
func maybeReachable(scope network.Scope) bool {
	switch scope {
	case network.ScopeCloudLocal:
		return true
	case network.ScopePublic:
		return true
	case "":
		return true
	default:
		return false
	}
}

func websocketURL(s string, mt names.ModelTag, finalPath string, attrs url.Values) string {
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
	u.RawQuery = attrs.Encode()
	return u.String()
}

// dialAll simultaneously dials all given urls and returns the first
// connection.
func dialAll(ctx context.Context, dialer *Dialer, urls []string, headers http.Header) (*websocket.Conn, error) {
	if len(urls) == 0 {
		return nil, errors.New("no urls to dial")
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
		wg.Go(func() {
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
		})
	}
	wg.Wait()
	if res != nil {
		return res, nil
	}
	return nil, err
}
