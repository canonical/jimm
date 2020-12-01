// Copyright 2020 Canonical Ltd.

// Package jimm contains the business logic used to manage clouds,
// cloudcredentials and models.
package jimm

import (
	"context"

	jujuparams "github.com/juju/juju/apiserver/params"

	"github.com/CanonicalLtd/jimm/internal/db"
	"github.com/CanonicalLtd/jimm/internal/dbmodel"
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
}

// An Authenticator authenticates login requests.
type Authenticator interface {
	// Authenticate processes the given LoginRequest and returns the user
	// that has authenticated.
	Authenticate(ctx context.Context, req *jujuparams.LoginRequest) (*dbmodel.User, error)
}
