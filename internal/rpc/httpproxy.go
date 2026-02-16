// Copyright 2025 Canonical.

package rpc

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"log"
	"math/rand/v2"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"github.com/juju/juju/core/network"
	"github.com/juju/names/v4"
	"github.com/juju/zaputil/zapctx"
	"go.uber.org/zap"
)

const (
	defaultScheme = "https"
)

// ConnectionDetails holds the details required to connect to a Juju controller.
type ConnectionDetails struct {
	// CACertificate is the CA certificate required to access this
	// controller. This is only set if the controller endpoint's
	// certificate is not signed by a public CA.
	CACertificate string
	// PublicAddress is the public address registered with the controller
	// when it was added. This address will normally be a resolvable DNS
	// name and port.
	PublicAddress string
	// TLSHostname provides a hostname that should be used for TLS verfication.
	// Useful for local dev to avoid TLS issues.
	TLSHostname string
	// Addresses holds the known addresses on which the controller is
	// listening.
	Addresses []network.MachineHostPorts
	// Username used to authentica with the controller.
	Username string
	// Password used to authenticate with the controller.
	Password string
}

// ProxyHTTP handles HTTP requests by proxying them to the Juju controller.
// It retrieves the controller's addresses, sets up TLS if necessary,
// and acts as a reverse proxy to forward the request.
func ProxyHTTP(ctx context.Context, ctl ConnectionDetails, w http.ResponseWriter, req *http.Request) {
	urls, err := getControllerAddresses(ctl)
	if err != nil {
		zapctx.Error(ctx, "failed to get controller addresses", zap.Error(err))
		http.Error(w, fmt.Sprintf("failed to get controller addresses: %v", err), http.StatusInternalServerError)
		return
	}

	if len(urls) == 0 {
		zapctx.Error(ctx, "no controller addresses found")
		http.Error(w, "no controller addresses found", http.StatusInternalServerError)
		return
	}

	// Shuffle for initial load balancing
	if len(urls) > 1 {
		rand.Shuffle(len(urls), func(i, j int) {
			urls[i], urls[j] = urls[j], urls[i]
		})
	}

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

	baseTransport := http.DefaultTransport.(*http.Transport).Clone()
	baseTransport.TLSClientConfig = tlsConfig

	// Create custom transport that handles multiple backends
	multiBackendTransport, err := newMultiBackendTransport(baseTransport, urls)
	if err != nil {
		zapctx.Error(ctx, "failed to create multiBackendTransport", zap.Error(err))
		http.Error(w, fmt.Sprintf("failed to create multiBackendTransport: %v", err), http.StatusInternalServerError)
		return
	}

	adminUsername := names.NewUserTag(ctl.Username).String()
	proxy := &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			// The multiBackendTransport will handle URL selection and failover
			// We just need to set up basic auth here
			pr.Out.SetBasicAuth(adminUsername, ctl.Password)
		},
		Transport: multiBackendTransport,
		ErrorLog:  log.New(&proxyErrorLogger{}, "", 0), // flag=0 to avoid printing extra info that zap already gives us
	}
	proxy.ServeHTTP(w, req)
}

type proxyErrorLogger struct{}

func (pl *proxyErrorLogger) Write(p []byte) (n int, err error) {
	zapctx.Error(context.Background(), "HTTP proxy error", zap.String("error", string(p)))
	return len(p), nil
}

func getControllerAddresses(ctl ConnectionDetails) ([]*url.URL, error) {
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
				if hp.Type == network.IPv6Address {
					ip = fmt.Sprintf("[%s]:%d", hp.Value, hp.Port())
				} else {
					ip = fmt.Sprintf("%s:%d", hp.Value, hp.Port())
				}
				newURL := url.URL{Scheme: defaultScheme, Host: ip}
				urls = append(urls, &newURL)
			}
		}
	}
	return urls, nil
}
