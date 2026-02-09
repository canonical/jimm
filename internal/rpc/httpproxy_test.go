// Copyright 2025 Canonical.

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
		_, _ = w.Write([]byte("OK"))
	}))
	defer fakeController.Close()
	pemData := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: fakeController.Certificate().Raw,
	})
	controllerCACert := string(pemData)
	fakeControllerURL, err := url.Parse(fakeController.URL)
	c.Assert(err, qt.IsNil)

	tests := []struct {
		description          string
		getConnectionDetails func(c *qt.C) rpc.ConnectionDetails
		path                 string
		statusExpected       int
	}{
		{
			description: "good",
			getConnectionDetails: func(c *qt.C) rpc.ConnectionDetails {

				return rpc.ConnectionDetails{
					CACertificate: controllerCACert,
					PublicAddress: fakeControllerURL.Host,
				}
			},
			statusExpected: http.StatusOK,
		}, {
			description: "controller no public address, only addresses",
			getConnectionDetails: func(c *qt.C) rpc.ConnectionDetails {
				hp, err := network.ParseMachineHostPort(fakeController.Listener.Addr().String())
				c.Assert(err, qt.Equals, nil)
				hp.Scope = network.ScopePublic

				hostPorts := [][]jujuparams.HostPort{{{
					Address: jujuparams.FromMachineAddress(hp.MachineAddress),
					Port:    hp.Port(),
				}}}
				controllerAddresses := jujuparams.ToMachineHostsPorts(hostPorts)
				return rpc.ConnectionDetails{
					CACertificate: controllerCACert,
					Addresses:     controllerAddresses,
				}
			},
			statusExpected: http.StatusOK,
		},
		{
			description: "controller public address with unreachable alternatives",
			getConnectionDetails: func(c *qt.C) rpc.ConnectionDetails {
				hp, err := network.ParseMachineHostPort("unreachable:61213")
				c.Assert(err, qt.Equals, nil)
				hp.Scope = network.ScopePublic

				hostPorts := [][]jujuparams.HostPort{{{
					Address: jujuparams.FromMachineAddress(hp.MachineAddress),
					Port:    hp.Port(),
				}}}
				controllerAddresses := jujuparams.ToMachineHostsPorts(hostPorts)
				return rpc.ConnectionDetails{
					PublicAddress: fakeController.Listener.Addr().String(),
					Addresses:     controllerAddresses,
					CACertificate: controllerCACert,
				}
			},
			statusExpected: http.StatusOK,
		},
		{
			description: "controller responds unauthorized",
			getConnectionDetails: func(c *qt.C) rpc.ConnectionDetails {
				return rpc.ConnectionDetails{
					PublicAddress: fakeControllerURL.Host,
					CACertificate: controllerCACert,
				}
			},
			path:           "/unauth",
			statusExpected: http.StatusUnauthorized,
		},
		{
			description: "controller not reachable",
			getConnectionDetails: func(c *qt.C) rpc.ConnectionDetails {
				return rpc.ConnectionDetails{
					PublicAddress: "localhost-not-found:61213",
					CACertificate: controllerCACert,
				}
			},
			statusExpected: http.StatusBadGateway,
		},
	}

	for _, test := range tests {
		t.Run(test.description, func(t *testing.T) {
			c := qt.New(t)

			req, err := http.NewRequest("POST", test.path, nil)
			c.Assert(err, qt.IsNil)
			recorder := httptest.NewRecorder()

			connectionDetails := test.getConnectionDetails(c)
			connectionDetails.Username = "test-user"
			connectionDetails.Password = "test-password"
			rpc.ProxyHTTP(ctx, connectionDetails, recorder, req)

			resp := recorder.Result()
			defer resp.Body.Close()
			c.Assert(resp.StatusCode, qt.Equals, test.statusExpected)
		})
	}
}
