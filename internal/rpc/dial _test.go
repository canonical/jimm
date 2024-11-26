// Copyright 2024 Canonical.

package rpc_test

import (
	"context"
	"encoding/pem"
	"net/http"
	"testing"

	qt "github.com/frankban/quicktest"
	"github.com/juju/juju/core/network"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v5"

	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/rpc"
)

func TestDialIPv4(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()
	fakeController := newServer(echo)
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
	fakeController := newIPv6Server(echo)
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
