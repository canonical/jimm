// Copyright 2025 Canonical.

package juju

// ControllerCreds represent the admin username and password
// used to authenticate with a Juju controller via basic auth.
type ControllerCreds struct {
	AdminIdentityName string
	AdminPassword     string
}
