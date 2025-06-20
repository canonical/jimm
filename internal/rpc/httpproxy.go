// Copyright 2025 Canonical.

package rpc

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	"github.com/juju/juju/core/network"
	"github.com/juju/names/v4"
	"github.com/juju/zaputil/zapctx"
	"go.uber.org/zap"

	"github.com/canonical/jimm/v3/internal/dbmodel"
)

const (
	defaultScheme = "https"
)

// ControllerDetails contains the details
// used to connect to a Juju controller
// as well as the admin user's credentials.
type ControllerDetails struct {
	Controller dbmodel.Controller
	Username   string
	Password   string
}

// ProxyHTTP handles HTTP requests by proxying them to the Juju controller.
// It retrieves the controller's addresses, sets up TLS if necessary,
// and acts as a reverse proxy to forward the request.
func ProxyHTTP(ctx context.Context, ctl ControllerDetails, w http.ResponseWriter, req *http.Request) {
	urls, err := getControllerAddresses(ctl.Controller)
	if err != nil {
		zapctx.Error(ctx, "failed to get controller addresses", zap.Error(err))
		http.Error(w, fmt.Sprintf("failed to get controller addresses: %v", err), http.StatusInternalServerError)
		return
	}

	if len(urls) == 0 {
		zapctx.Error(ctx, "no controller addresses found", zap.String("controller", ctl.Controller.Name))
		http.Error(w, "no controller addresses found", http.StatusInternalServerError)
		return
	}

	var tlsConfig *tls.Config
	if ctl.Controller.CACertificate != "" {
		cp := x509.NewCertPool()
		ok := cp.AppendCertsFromPEM([]byte(ctl.Controller.CACertificate))
		if !ok {
			zapctx.Warn(ctx, "no CA certificates added")
		}
		tlsConfig = &tls.Config{
			RootCAs:    cp,
			ServerName: ctl.Controller.TLSHostname,
			MinVersion: tls.VersionTLS12,
		}
	}

	// transport is the default HTTP transport config with TLS configuration.
	transport := &http.Transport{
		TLSClientConfig:       tlsConfig,
		Proxy:                 http.ProxyFromEnvironment,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	if len(urls) > 1 {
		rand.Shuffle(len(urls), func(i, j int) {
			urls[i], urls[j] = urls[j], urls[i]
		})
	}

	// TODO: Consider implementing a better load balancing mechanism that handles
	// multiples URLs and handles failing backends gracefully e.g. try send to first
	// URL and on failure, try second, etc.
	proxy := &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(urls[0])
			pr.Out.SetBasicAuth(names.NewUserTag(ctl.Username).String(), ctl.Password)
		},
		Transport: transport,
		ErrorLog:  log.New(&proxyErrorLogger{}, "", 0), // flag=0 to avoid printing extra info that zap already gives us
	}
	proxy.ServeHTTP(w, req)
}

type proxyErrorLogger struct{}

func (pl *proxyErrorLogger) Write(p []byte) (n int, err error) {
	zapctx.Error(context.Background(), "HTTP proxy error", zap.String("error", string(p)))
	return len(p), nil
}

func getControllerAddresses(ctl dbmodel.Controller) ([]*url.URL, error) {
	urls := make([]*url.URL, 0, 1)
	if ctl.PublicAddress != "" {
		address := ctl.PublicAddress
		if !strings.Contains(address, "://") {
			address = defaultScheme + "://" + address // ensure the address has a scheme
		}
		newURL, err := url.Parse(address)
		if err != nil {
			return nil, err
		}
		urls = append(urls, newURL)
		return urls, nil
	}

	for _, hps := range ctl.Addresses {
		for _, hp := range hps {
			if maybeReachable(hp.Scope) {
				var ip string
				if hp.Type == string(network.IPv6Address) {
					ip = fmt.Sprintf("[%s]:%d", hp.Value, hp.Port)
				} else {
					ip = fmt.Sprintf("%s:%d", hp.Value, hp.Port)
				}
				newURL := url.URL{Scheme: defaultScheme, Host: ip}
				urls = append(urls, &newURL)
			}
		}
	}
	return urls, nil
}
