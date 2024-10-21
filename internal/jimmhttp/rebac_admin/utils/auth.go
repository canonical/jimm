// Copyright 2024 Canonical.

package utils

import (
	"context"
	"errors"

	rebac_handlers "github.com/canonical/rebac-admin-ui-handlers/v1"

	"github.com/canonical/jimm/v3/internal/openfga"
)

// GetUserFromContext retrieves the OpenFGA user pointer from the context
// returning an error if it does not exist or is not the correct type.
func GetUserFromContext(ctx context.Context) (*openfga.User, error) {
	raw, err := rebac_handlers.GetIdentityFromContext(ctx)
	if err != nil {
		return nil, err
	}
	user, ok := raw.(*openfga.User)
	if !ok {
		return nil, errors.New("unable to fetch authenticated user")
	}
	return user, nil
}
