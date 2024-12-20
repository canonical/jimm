// Copyright 2024 Canonical.
package login

import (
	"context"

	"github.com/canonical/jimm/v3/internal/openfga"
)

// Login is a type alias to export loginManager for use in tests.
type LoginManager = loginManager

func (j *LoginManager) GetOrCreateUser(ctx context.Context, identifier string) (*openfga.User, error) {
	return j.getOrCreateUser(ctx, identifier)
}
