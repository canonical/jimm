// Copyright 2021 canonical.

package auth

import (
	jujuparams "github.com/juju/juju/rpc/params"
)

// An AuthenticationError is the error returned when the requested
// authentication has failed.
type AuthenticationError struct {
	// LoginResult may contain a login challenge to send to the client.
	LoginResult jujuparams.LoginResult
}

// Error implements the error interface.
func (*AuthenticationError) Error() string {
	return "authentication failed"
}
