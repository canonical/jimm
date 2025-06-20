// Copyright 2025 Canonical.

package juju

import "github.com/canonical/jimm/v3/internal/dbmodel"

// ControllerCreds represent the admin username and password
// used to authenticate with a Juju controller via basic auth.
type ControllerCreds struct {
	AdminIdentityName string
	AdminPassword     string
}

// ControllerDetails contains the details of a Juju controller,
// including the controller itself and the credentials used to access it.
type ControllerDetails struct {
	Controller  dbmodel.Controller
	Credentials ControllerCreds
}
