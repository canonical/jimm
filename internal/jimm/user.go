// Copyright 2020 Canonical Ltd.

package jimm

import (
	"context"

	"github.com/canonical/jimm/v3/internal/db"
	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/openfga"
)

// GetUser fetches the user specified by the user's email or the service accounts ID
// and returns an openfga User that can be used to verify user's permissions.
func (j *JIMM) GetUser(ctx context.Context, identifier string) (*openfga.User, error) {
	const op = errors.Op("jimm.GetUser")

	user, err := dbmodel.NewIdentity(identifier)
	if err != nil {
		return nil, errors.E(op, err)
	}

	if err := j.Database.GetIdentity(ctx, user); err != nil {
		return nil, err
	}
	u := openfga.NewUser(user, j.OpenFGAClient)

	isJimmAdmin, err := openfga.IsAdministrator(ctx, u, j.ResourceTag())
	if err != nil {
		return nil, errors.E(op, err)
	}
	u.JimmAdmin = isJimmAdmin

	return u, nil
}

func (j *JIMM) UpdateUserLastLogin(ctx context.Context, identifier string) error {
	const op = errors.Op("jimm.UpdateUserLastLogin")
	user, err := dbmodel.NewIdentity(identifier)
	if err != nil {
		return err
	}
	if err := j.Database.Transaction(func(tx *db.Database) error {
		if err := tx.GetIdentity(ctx, user); err != nil {
			return err
		}
		user.LastLogin.Time = j.Database.DB.Config.NowFunc()
		user.LastLogin.Valid = true
		return tx.UpdateIdentity(ctx, user)
	}); err != nil {
		return errors.E(op, err)
	}
	return nil
}
