// Copyright 2025 Canonical.

package jimm

// ControllerCreds represent the admin username and password
// used to authenticate with a Juju controller via basic auth.
type ControllerCreds struct {
	AdminIdentityName string
	AdminPassword     string
}
