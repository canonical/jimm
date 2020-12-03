// Copyright 2020 Canonical Ltd.

// Package jimm contains the business logic used to manage clouds,
// cloudcredentials and models.
package jimm

import (
	"context"

	jujuparams "github.com/juju/juju/apiserver/params"

	"github.com/CanonicalLtd/jimm/internal/db"
	"github.com/CanonicalLtd/jimm/internal/dbmodel"
	"github.com/CanonicalLtd/jimm/internal/errors"
)

// A JIMM provides the buisness logic for managing resources in the JAAS
// system. A single JIMM instance is shared by all concurrent API
// connections therefore the JIMM object itself does not contain any per-
// request state.
type JIMM struct {
	// Database is the satabase used by JIMM, this provides direct access
	// to the data store. Any client accessing the database directly is
	// responsible for ensuring that the authenticated user has access to
	// the data.
	Database db.Database

	// Authenticator is the authenticator JIMM uses to determine the user
	// authenticating with the API. If this is not specified then all
	// authentication requests are considered to have failed.
	Authenticator Authenticator

	// Dialer is the API dialer JIMM uses to contact juju controllers. if
	// this is not configured all connection attempts will fail.
	Dialer Dialer
}

// An Authenticator authenticates login requests.
type Authenticator interface {
	// Authenticate processes the given LoginRequest and returns the user
	// that has authenticated.
	Authenticate(ctx context.Context, req *jujuparams.LoginRequest) (*dbmodel.User, error)
}

// dial dials the controller and model specified by the given Controller
// and ModelTag. If no Dialer has been configured then an error with a
// code of CodeConnectionFailed will be returned.
func (j *JIMM) dial(ctx context.Context, ctl *dbmodel.Controller, modelTag string) (API, error) {
	if j == nil || j.Dialer == nil {
		return nil, errors.E(errors.CodeConnectionFailed, "no dialer configured")
	}
	return j.Dialer.Dial(ctx, ctl, modelTag)
}

// A Dialer provides a connection to a controller.
type Dialer interface {
	// Dial creates an API connection to a controller. If the given
	// model-tag is non-zero the connection will be to that model,
	// otherwise the connection is to the controller. After sucessfully
	// dialing the controller the UUID, AgentVersion and HostPorts fields
	// in the given controller should be updated to the values provided
	// by the controller.
	Dial(ctx context.Context, ctl *dbmodel.Controller, modelTag string) (API, error)
}

// An API is the interface JIMM uses to access the API on a controller.
type API interface {
	// Close closes the API connection.
	Close()

	// CloudInfo fetches the cloud information for the cloud with the given
	// tag.
	CloudInfo(ctx context.Context, tag string, ci *jujuparams.CloudInfo) error

	// Clouds returns the set of clouds supported by the controller.
	Clouds(context.Context) (map[string]jujuparams.Cloud, error)

	// ControllerModelSummary fetches the model summary of the model on the
	// controller that hosts the controller machines.
	ControllerModelSummary(context.Context, *jujuparams.ModelSummary) error
}
