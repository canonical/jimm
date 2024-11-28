// Copyright 2024 Canonical.
package jujuclient2

import (
	"context"
	"encoding/base64"
	"fmt"
	"net"
	"net/http"
	"strconv"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/connector"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v5"

	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/jimmjwx"
)

type JWTLoginProvider struct {
	Permissions   map[string]string
	ModelTag      names.ModelTag
	ControllerTag names.ControllerTag
	JWTService    *jimmjwx.JWTService
}

func (jlp *JWTLoginProvider) createLoginRequest(ctx context.Context) (*jujuparams.LoginRequest, error) {
	// JIMM is automatically given all required permissions
	permissions := jlp.Permissions
	if permissions == nil {
		permissions = make(map[string]string)
	}
	permissions[jlp.ControllerTag.String()] = "superuser"
	if jlp.ModelTag.Id() != "" {
		permissions[jlp.ModelTag.String()] = "admin"
	}

	jwt, err := jlp.JWTService.NewJWT(ctx, jimmjwx.JWTParams{
		Controller: jlp.ControllerTag.Id(),
		User:       names.NewUserTag("admin").String(),
		Access:     permissions,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to build jwt: %w", err)
	}
	jwtString := base64.StdEncoding.EncodeToString(jwt)

	return &jujuparams.LoginRequest{
		AuthTag: names.NewUserTag("admin").String(),
		Token:   jwtString,
	}, nil
}

// Implements juju/api.LoginProvider.Login.
//
// Login performs log in when connecting to the controller.
func (jlp *JWTLoginProvider) Login(ctx context.Context, caller base.APICaller) (*api.LoginResultParams, error) {
	loginReq, err := jlp.createLoginRequest(ctx)
	if err != nil {
		return nil, err
	}

	var loginRes jujuparams.LoginResult
	if err := caller.APICall("Admin", 3, "", "Login", loginReq, &loginRes); err != nil {
		return nil, err
	}

	loginResParams, err := api.NewLoginResultParams(loginRes)
	if err != nil {
		return nil, err
	}

	return loginResParams, nil
}

// Implements juju/api.LoginProvider.AuthHeader.
//
// AuthHeader returns an HTTP header used for authentication.
// This is normally used as part of basic authentication in scenarios where a client
// makes use of a StreamConnector like when fetching logs using `juju debug-log`.
// Can return [ErrorLoginFirst] when the provider requires an RPC login before basic auth
// can be performed.
// Other errors are also possible indicating an internal error in the provider.
func (jlp *JWTLoginProvider) AuthHeader() (http.Header, error) {
	return nil, nil
}

type JujuDialer struct {
	JWTService *jimmjwx.JWTService
}

// TODO(ale8k): Don't pass a dbmodel.Controller, create params for this.
func (jd *JujuDialer) Dial(ctl *dbmodel.Controller, modelTag names.ModelTag, requiredPermissions map[string]string) (api.Connection, error) {
	cfg := connector.SimpleConfig{}
	if ctl.PublicAddress != "" {
		cfg.ControllerAddresses = []string{ctl.PublicAddress}
	} else {
		for _, hps := range ctl.Addresses {
			for _, hp := range hps {
				cfg.ControllerAddresses = append(cfg.ControllerAddresses, net.JoinHostPort(hp.Value, strconv.Itoa(hp.Port)))
			}
		}
	}

	if ctl.CACertificate != "" {
		cfg.CACert = ctl.CACertificate
	}
	if modelTag.Id() != "" {
		cfg.ModelUUID = modelTag.Id()
	}
	// TODO(ale8k):
	// Required for now as the simple config checks for user/clientid
	// Perhaps have a separate validation method that is run after creation
	// of SimpleConnector.
	cfg.Username = "placeholder"

	loginProvider := &JWTLoginProvider{
		JWTService:    jd.JWTService,
		ControllerTag: ctl.ResourceTag(),
		ModelTag:      modelTag,
		Permissions:   requiredPermissions,
	}
	connector, err := connector.NewSimple(
		cfg,
		api.WithLoginProvider(loginProvider),
	)
	if err != nil {
		return nil, err
	}

	connection, err := connector.Connect()
	if err != nil {
		return nil, err
	}

	return connection, nil
}
