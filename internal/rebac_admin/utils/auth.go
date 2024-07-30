package utils

import (
	"context"
	"errors"

	"github.com/canonical/jimm/v3/internal/openfga"
	rebac_handlers "github.com/canonical/rebac-admin-ui-handlers/v1"
)

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
