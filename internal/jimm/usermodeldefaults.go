// Copyright 2020 Canonical Ltd.

package jimm

import (
	"context"

	"github.com/canonical/jimm/internal/dbmodel"
	"github.com/canonical/jimm/internal/errors"
)

// SetUserModelDefaults writes new default model setting values for the user.
func (j *JIMM) SetUserModelDefaults(ctx context.Context, user *dbmodel.Identity, configs map[string]interface{}) error {
	const op = errors.Op("jimm.SetUserModelDefaults")

	for k := range configs {
		if k == agentVersionKey {
			return errors.E(op, errors.CodeBadRequest, `agent-version cannot have a default value`)
		}
	}

	err := j.Database.SetUserModelDefaults(ctx, &dbmodel.UserModelDefaults{
		Username: user.Username,
		Defaults: configs,
	})
	if err != nil {
		return errors.E(op, err)
	}
	return nil
}

// UserModelDefaults returns the default config values for the user.
func (j *JIMM) UserModelDefaults(ctx context.Context, user *dbmodel.Identity) (map[string]interface{}, error) {
	const op = errors.Op("jimm.UserModelDefaults")

	defaults := dbmodel.UserModelDefaults{
		Username: user.Username,
	}
	err := j.Database.UserModelDefaults(ctx, &defaults)
	if err != nil {
		return nil, errors.E(op, err)
	}

	return defaults.Defaults, nil
}
