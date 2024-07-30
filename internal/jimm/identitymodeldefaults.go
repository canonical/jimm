// Copyright 2020 Canonical Ltd.

package jimm

import (
	"context"

	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
)

// SetIdentityModelDefaults writes new default model setting values for the user.
func (j *JIMM) SetIdentityModelDefaults(ctx context.Context, identity *dbmodel.Identity, configs map[string]interface{}) error {
	const op = errors.Op("jimm.SetIdentityModelDefaults")

	for k := range configs {
		if k == agentVersionKey {
			return errors.E(op, errors.CodeBadRequest, `agent-version cannot have a default value`)
		}
	}

	err := j.Database.SetIdentityModelDefaults(ctx, &dbmodel.IdentityModelDefaults{
		IdentityName: identity.Name,
		Defaults:     configs,
	})
	if err != nil {
		return errors.E(op, err)
	}
	return nil
}

// IdnetityModelDefaults returns the default config values for the identity.
func (j *JIMM) IdentityModelDefaults(ctx context.Context, identity *dbmodel.Identity) (map[string]interface{}, error) {
	const op = errors.Op("jimm.UserModelDefaults")

	defaults := dbmodel.IdentityModelDefaults{
		IdentityName: identity.Name,
	}
	err := j.Database.IdentityModelDefaults(ctx, &defaults)
	if err != nil {
		return nil, errors.E(op, err)
	}

	return defaults.Defaults, nil
}
