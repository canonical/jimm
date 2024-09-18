// Copyright 2024 Canonical.
package rpc_test

import (
	"context"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	qt "github.com/frankban/quicktest"
	"github.com/juju/juju/core/network"
	jujuparams "github.com/juju/juju/rpc/params"

	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/rpc"
)

func TestProxyHTTP(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()
	// we expect the controller to respond with TLS
	fakeController := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.String(), "unauth") {
			w.WriteHeader(401)
			return
		}
		_, err := w.Write([]byte("OK"))
		c.Assert(err, qt.IsNil)
	}))
	defer fakeController.Close()
	controller := dbmodel.Controller{}
	pemData := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: fakeController.Certificate().Raw,
	})
	controller.CACertificate = string(pemData)

	tests := []struct {
		description    string
		setup          func()
		path           string
		statusExpected int
	}{
		{
			description: "good",
			setup: func() {
				newURL, _ := url.Parse(fakeController.URL)
				controller.PublicAddress = newURL.Host
			},
			statusExpected: http.StatusOK,
		},
		{
			description: "controller no public address, only addresses",
			setup: func() {
				hp, err := network.ParseMachineHostPort(fakeController.Listener.Addr().String())
				c.Assert(err, qt.Equals, nil)
				controller.Addresses = append(make([][]jujuparams.HostPort, 0), []jujuparams.HostPort{{
					Address: jujuparams.FromMachineAddress(hp.MachineAddress),
					Port:    hp.Port(),
				}})
				controller.Addresses = append(controller.Addresses, []jujuparams.HostPort{})
				controller.PublicAddress = ""
			},
			statusExpected: http.StatusOK,
		},
		{
			description: "controller no public address, only addresses",
			setup: func() {
				hp, err := network.ParseMachineHostPort(fakeController.Listener.Addr().String())
				c.Assert(err, qt.Equals, nil)
				controller.Addresses = append(make([][]jujuparams.HostPort, 0), []jujuparams.HostPort{{
					Address: jujuparams.FromMachineAddress(hp.MachineAddress),
					Port:    hp.Port(),
				}})
				controller.Addresses = append(controller.Addresses, []jujuparams.HostPort{})
				controller.PublicAddress = ""
			},
			statusExpected: http.StatusOK,
		},
		{
			description: "controller responds unauthorized",
			setup: func() {
				newURL, _ := url.Parse(fakeController.URL)
				controller.PublicAddress = newURL.Host
			},
			path:           "/unauth",
			statusExpected: http.StatusUnauthorized,
		},
		{
			description: "controller not reachable",
			setup: func() {
				controller.Addresses = nil
				controller.PublicAddress = "localhost-not-found:61213"
			},
			statusExpected: http.StatusInternalServerError,
		},
	}

	for _, test := range tests {
		test.setup()
		req, err := http.NewRequest("POST", test.path, nil)
		c.Assert(err, qt.IsNil)
		recorder := httptest.NewRecorder()
		rpc.ProxyHTTP(ctx, &controller, recorder, req)
		resp := recorder.Result()
		defer resp.Body.Close()
		c.Assert(resp.StatusCode, qt.Equals, test.statusExpected)
	}
}
