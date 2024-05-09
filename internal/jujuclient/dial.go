// Copyright 2020 Canonical Ltd.

// Package jujuclient is the client JIMM uses to connect to juju
// controllers. The jujuclient uses the juju RPC API directly using
// API-native types, mostly those coming from github.com/juju/names and
// github.com/juju/juju/apiserver/params. The rationale for this being that
// as JIMM both sends and receives messages across this API it should
// perform as little format conversion as possible.
package jujuclient

import (
	"context"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/connector"
	"github.com/juju/names/v5"

	"github.com/canonical/jimm/internal/dbmodel"
	"github.com/canonical/jimm/internal/errors"
	"github.com/canonical/jimm/internal/jimm"
	"github.com/canonical/jimm/internal/jimmjwx"
)

func getJujuApiConnection(config connector.SimpleConfig, dialOpts func(do *api.DialOpts)) (api.Connection, error) {
	connr, err := connector.NewSimple(config, dialOpts)
	if err != nil {
		return nil, err
	}
	connr.Info.SkipLogin = false

	conn, err := connr.Connect()
	return conn, err
}

func getAddrAndPort(addr string) (string, int, error) {
	addrSplit := strings.Split(addr, ":")
	addrAddr := addrSplit[0]
	addrPort, err := strconv.Atoi(addrSplit[1])
	return addrAddr, addrPort, err
}

// A Dialer is an implementation of a jimm.Dialer that adapts a juju API
// connection to provide a jimm API.
type Dialer struct {
	JWTService *jimmjwx.JWTService
}

func (d *Dialer) getJwt(ctx context.Context, ctl *dbmodel.Controller, modelTag names.ModelTag, p map[string]string) (string, error) {
	// JIMM is automatically given all required permissions
	permissions := p
	if permissions == nil {
		permissions = make(map[string]string)
	}
	permissions[ctl.ResourceTag().String()] = "superuser"
	if modelTag.Id() != "" {
		permissions[modelTag.String()] = "admin"
	}

	jwt, err := d.JWTService.NewJWT(ctx, jimmjwx.JWTParams{
		Controller: ctl.UUID,
		User:       names.NewUserTag("admin").String(),
		Access:     permissions,
	})
	if err != nil {
		return "", errors.E(err)
	}
	jwtString := base64.StdEncoding.EncodeToString(jwt)

	return jwtString, nil
}

// Dial implements jimm.Dialer.
func (d *Dialer) Dial(ctx context.Context, ctl *dbmodel.Controller, modelTag names.ModelTag, requiredPermissions map[string]string) (jimm.API, error) {
	const op = errors.Op("jujuclient.Dial")

	// Parse the controller host ports into a normal api address string format
	apiAddresses := make([]string, 0)
	for _, hps := range ctl.Addresses {
		for _, hp := range hps {
			apiAddresses = append(apiAddresses, fmt.Sprintf("%s:%d", hp.Value, hp.Port))
		}
	}

	jwt, err := d.getJwt(
		ctx,
		ctl,
		modelTag,
		requiredPermissions,
	)
	if err != nil {
		//TODO
	}

	authTag := names.NewUserTag("admin").String()

	dialOptions := func(do *api.DialOpts) {
		//this is set as a const above, in case we need to use it elsewhere to manage connection timings
		do.Timeout = 0
		//default is 2 seconds, as we are changing the overall timeout it makes sense to reduce this as well
		do.RetryDelay = 1 * time.Second
		// IDK what im even doing
		do.LoginProvider = api.NewJWTLoginProvider(
			authTag,
			jwt,
		)
	}

	jujuConn, err := getJujuApiConnection(connector.SimpleConfig{
		ControllerAddresses: apiAddresses,
		Username:            ctl.AdminIdentityName,
		CACert:              ctl.CACertificate,
		ModelUUID:           modelTag.Id(),
	}, dialOptions)
	if err != nil {
		return nil, errors.E(op, err)
	}

	serverVersion, versionSet := jujuConn.ServerVersion()
	if versionSet == false {
		// What do we do? This means server reported 0
		// and as such it isn't set... Otherwise all is OK.
	}
	ctl.AgentVersion = serverVersion.String()

	c := Connection{
		jujuConn,
		authTag,
	}
	return c, nil
}

// Ale8k: We try v2 this shizz
type Connection struct {
	api.Connection
	// Preventing refactor but probably doesn't belong here
	// this should be stored on a struct dedicated to user connections
	// upon a successful dial, not in "Connection", which so happens
	// to also be an implementation of jimm.API
	userTag string
}

func (c *Connection) Call(
	ctx context.Context,
	facade string,
	version int,
	id,
	method string,
	args,
	resp interface{},
) error {
	return c.APICall(facade, version, id, method, args, resp)
}

func (c *Connection) CallHighestFacadeVersion(
	ctx context.Context,
	facade string,
	versions []int,
	id,
	method string,
	args,
	resp interface{},
) error {
	bestVersion := c.BestFacadeVersion(facade)
	return c.APICall(
		facade,
		bestVersion,
		"",
		method,
		args,
		resp,
	)
}

// not needed anymore but just preventing a refactor for now
func (c *Connection) hasFacadeVersion(facade string, version int) bool {
	return c.Connection.BestFacadeVersion(facade) >= version
}
