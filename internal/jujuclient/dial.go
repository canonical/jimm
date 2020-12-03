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
	"time"

	"github.com/juju/juju/api"
	"github.com/juju/names/v4"

	"github.com/CanonicalLtd/jimm/internal/dbmodel"
	"github.com/CanonicalLtd/jimm/internal/errors"
	"github.com/CanonicalLtd/jimm/internal/jimm"
)

// A Dialer is an implementation of a jimm.Dialer that adapts a juju API
// connection to provide a jimm API.
type Dialer struct{}

// Dial implements jimm.Dialer.
func (Dialer) Dial(ctx context.Context, ctl *dbmodel.Controller, modelTag names.ModelTag) (jimm.API, error) {
	const op = errors.Op("jujuclient.Dial")

	info := api.Info{
		CACert:   ctl.CACertificate,
		ModelTag: modelTag,
		Tag:      names.NewUserTag(ctl.AdminUser),
		Password: ctl.AdminPassword,
	}
	if ctl.PublicAddress != "" {
		info.Addrs = []string{ctl.PublicAddress}
	}
	info.Addrs = append(info.Addrs, ctl.Addresses...)
	dialOpts := api.DialOpts{}
	if dl, ok := ctx.Deadline(); ok {
		t := time.Now()
		if !t.Before(dl) {
			return nil, errors.E(op, errors.CodeConnectionFailed, ctx.Err())
		}
		dialOpts.Timeout = dl.Sub(t)
	}
	conn, err := api.Open(&info, dialOpts)
	if err != nil {
		return nil, errors.E(op, errors.CodeConnectionFailed, err)
	}
	ctl.SetTag(conn.ControllerTag())
	v, ok := conn.ServerVersion()
	if ok {
		ctl.AgentVersion = v.String()
	}
	for _, hps := range conn.APIHostPorts() {
		for _, hp := range hps {
			ctl.Addresses = append(ctl.Addresses, hp.String())
		}
	}
	return Connection{conn: conn}, nil
}

// A Connection is a connection to a juju controller. Connection methods
// are generally thin wrappers around juju RPC calls, although there are
// some more JIMM specific operations. The RPC calls prefer to use the
// earliest facade versions that support all the required data, but will
// fall-back to earlier versions with slightly degraded functionality if
// possible.
type Connection struct {
	conn api.Connection
}

// Close closes the connection.
func (c Connection) Close() error {
	return c.conn.Close()
}

// hasFacadeVersion returns whether the connection supports the given
// facade at the given version.
func (c Connection) hasFacadeVersion(facade string, version int) bool {
	for _, v := range c.conn.AllFacadeVersions()[facade] {
		if v == version {
			return true
		}
	}
	return false
}
