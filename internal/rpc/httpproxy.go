// Copyright 2024 Canonical.

package rpc

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/juju/zaputil"
	"github.com/juju/zaputil/zapctx"

	"github.com/canonical/jimm/v3/internal/dbmodel"
)

type httpOptions struct {
	TLSConfig *tls.Config
	URL       url.URL
}

// ProxyHTTP proxies the request to the controller using the info contained in dbmodel.Controller.
func ProxyHTTP(ctx context.Context, ctl *dbmodel.Controller, w http.ResponseWriter, req *http.Request) {
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

	if ctl.PublicAddress != "" {
		err := doRequest(ctx, w, req, httpOptions{
			TLSConfig: tlsConfig,
			URL:       createURLWithNewHost(*req.URL, ctl.PublicAddress),
		})
		if err == nil {
			return
		}
	}
	for _, hps := range ctl.Addresses {
		for _, hp := range hps {
			err := doRequest(ctx, w, req, httpOptions{
				TLSConfig: tlsConfig,
				URL:       createURLWithNewHost(*req.URL, fmt.Sprintf("%s:%d", hp.Value, hp.Port)),
			})
			if err == nil {
				return
			} else {
				zapctx.Error(ctx, "failed to proxy request: continue to next addr", zaputil.Error(err))
			}
		}
	}

	zapctx.Error(ctx, "couldn't find a valid address for controller")
	http.Error(w, "Gateway timeout", http.StatusGatewayTimeout)
}

func doRequest(ctx context.Context, w http.ResponseWriter, req *http.Request, opt httpOptions) error {
	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: opt.TLSConfig,
		},
	}
	req = req.Clone(ctx)
	req.RequestURI = ""
	req.URL = &opt.URL
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// copy headers
	for k, vv := range resp.Header {
		for _, v := range vv {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	// copy body
	_, err = io.Copy(w, resp.Body)
	if err != nil {
		return err
	}
	return nil
}

// createURLWithNewHost takes a url.URL as parameter and return a url.URL with new host set and https enforced.
func createURLWithNewHost(reqUrl url.URL, host string) url.URL {
	reqUrl.Scheme = "https"
	reqUrl.Host = host
	return reqUrl
}
