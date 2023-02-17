// Copyright 2020 Canonical Ltd.

package jimmtest

import (
	"context"

	jujuparams "github.com/juju/juju/rpc/params"

	"github.com/CanonicalLtd/jimm/internal/openfga"
)

// An Authenticator is an implementation of jimm.Authenticator that returns
// the stored user and error.
type Authenticator struct {
	User *openfga.User
	Err  error
}

// Authenticate implements jimm.Authenticator.
func (a Authenticator) Authenticate(_ context.Context, _ *jujuparams.LoginRequest) (*openfga.User, error) {
	return a.User, a.Err
}
