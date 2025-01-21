// Copyright 2025 Canonical.

package rpc_test

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"net/http"
	"testing"

	qt "github.com/frankban/quicktest"
	"github.com/juju/juju/core/network"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v5"

	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/rpc"
	"github.com/canonical/jimm/v3/internal/testutils/rpctest"
)

func TestDialIPv4(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()
	fakeController := newServer(rpctest.Echo)
	defer fakeController.Close()
	controller := dbmodel.Controller{}
	pemData := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: fakeController.Certificate().Raw,
	})
	controller.CACertificate = string(pemData)
	hp, err := network.ParseMachineHostPort(fakeController.Listener.Addr().String())
	c.Assert(err, qt.Equals, nil)
	controller.Addresses = append(make([][]jujuparams.HostPort, 0), []jujuparams.HostPort{{
		Address: jujuparams.Address{
			Value: hp.Value,
			Type:  "ipv4",
		},
		Port: hp.Port(),
	}})
	_, err = rpc.Dial(ctx, &controller, names.ModelTag{}, "", http.Header{})
	c.Assert(err, qt.Equals, nil)
}

func TestDialIPv6(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()
	fakeController := newIPv6Server(rpctest.Echo)
	defer fakeController.Close()
	controller := dbmodel.Controller{}
	pemData := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: fakeController.Certificate().Raw,
	})
	controller.CACertificate = string(pemData)
	hp, err := network.ParseMachineHostPort(fakeController.Listener.Addr().String())
	c.Assert(err, qt.Equals, nil)
	controller.Addresses = append(make([][]jujuparams.HostPort, 0), []jujuparams.HostPort{{
		Address: jujuparams.Address{
			Value: hp.Value,
			Type:  "ipv6",
		},
		Port: hp.Port(),
	}})
	_, err = rpc.Dial(ctx, &controller, names.ModelTag{}, "", http.Header{})
	c.Assert(err, qt.Equals, nil)
}

func TestGetAddressesAndTLSConfig(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()

	tests := []struct {
		name           string
		controller     dbmodel.Controller
		expectedAddrs  []string
		expectedTLSCfg *tls.Config
	}{
		{
			name: "With CACertificate and PublicAddress",
			controller: dbmodel.Controller{
				CACertificate: "test-ca-cert",
				PublicAddress: "public.address.com",
				TLSHostname:   "tls.hostname.com",
			},
			expectedAddrs: []string{"public.address.com"},
			expectedTLSCfg: &tls.Config{
				RootCAs:    x509.NewCertPool(),
				ServerName: "tls.hostname.com",
				MinVersion: tls.VersionTLS12,
			},
		},
		{
			name: "With IPv4 Address",
			controller: dbmodel.Controller{
				Addresses: [][]jujuparams.HostPort{
					{
						{
							Address: jujuparams.Address{
								Value: "192.168.1.1",
								Type:  "ipv4",
							},
							Port: 8080,
						},
						{
							Address: jujuparams.Address{
								Value: "192.168.1.1",
								Type:  "ipv4",
								Scope: "non-exisiting-scope",
							},
							Port: 8080,
						},
					},
				},
			},
			expectedAddrs:  []string{"192.168.1.1:8080"},
			expectedTLSCfg: nil,
		},
		{
			name: "With IPv6 Address",
			controller: dbmodel.Controller{
				Addresses: [][]jujuparams.HostPort{
					{
						{
							Address: jujuparams.Address{
								Value: "2001:0db8:85a3:0000:0000:8a2e:0370:7334",
								Type:  "ipv6",
							},
							Port: 8080,
						},
						{
							Address: jujuparams.Address{
								Value: "2001:0db8:85a3:0000:0000:8a2e:0370:7335",
								Type:  "ipv6",
								Scope: string(network.ScopePublic),
							},
							Port: 8080,
						},
					},
				},
			},
			expectedAddrs:  []string{"[2001:0db8:85a3:0000:0000:8a2e:0370:7334]:8080", "[2001:0db8:85a3:0000:0000:8a2e:0370:7335]:8080"},
			expectedTLSCfg: nil,
		},
		{
			name: "With Both IPv4 and IPv6 Addresses",
			controller: dbmodel.Controller{
				Addresses: [][]jujuparams.HostPort{
					{
						{
							Address: jujuparams.Address{
								Value: "192.168.1.1",
								Type:  "ipv4",
							},
							Port: 8080,
						},
						{
							Address: jujuparams.Address{
								Value: "2001:0db8:85a3:0000:0000:8a2e:0370:7334",
								Type:  "ipv6",
							},
							Port: 8080,
						},
					},
				},
			},
			expectedAddrs:  []string{"192.168.1.1:8080", "[2001:0db8:85a3:0000:0000:8a2e:0370:7334]:8080"},
			expectedTLSCfg: nil,
		},
		{
			name: "No Addresses",
			controller: dbmodel.Controller{
				Addresses: [][]jujuparams.HostPort{},
			},
			expectedAddrs:  nil,
			expectedTLSCfg: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			addrs, tlsCfg := rpc.GetAddressesAndTLSConfig(ctx, &tt.controller)
			c.Assert(addrs, qt.DeepEquals, tt.expectedAddrs)
			if tt.expectedTLSCfg != nil {
				c.Assert(tlsCfg, qt.Not(qt.IsNil))
				c.Assert(tlsCfg.ServerName, qt.Equals, tt.expectedTLSCfg.ServerName)
				c.Assert(tlsCfg.MinVersion, qt.Equals, tt.expectedTLSCfg.MinVersion)
			} else {
				c.Assert(tlsCfg, qt.IsNil)
			}
		})
	}
}
