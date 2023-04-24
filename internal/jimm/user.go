// Copyright 2020 Canonical Ltd.

package jimm

import (
	"context"

	jujuparams "github.com/juju/juju/rpc/params"

	"github.com/CanonicalLtd/jimm/internal/db"
	"github.com/CanonicalLtd/jimm/internal/dbmodel"
	"github.com/CanonicalLtd/jimm/internal/errors"
	"github.com/CanonicalLtd/jimm/internal/openfga"
)

// Authenticate processes the given LoginRequest using the configured
// authenticator, it then retrieves the user information from the database.
// If the authenticated user does not yet exist in the database it will be
// created using the values returned from the authenticator as the user's
// details. If the authenticator returns a user with ControllerAccess set
// to "superuser" then the authenticated user will be considered a
// superuser for this session, this will not be persisted.
func (j *JIMM) Authenticate(ctx context.Context, req *jujuparams.LoginRequest) (*openfga.User, error) {
	const op = errors.Op("jimm.Authenticate")
	if j == nil || j.Authenticator == nil {
		return nil, errors.E(op, errors.CodeServerConfiguration, "authenticator not configured")
	}

	u, err := j.Authenticator.Authenticate(ctx, req)
	if err != nil {
		return nil, errors.E(op, err)
	}

	err = j.Database.Transaction(func(tx *db.Database) error {
		pu := dbmodel.User{
			Username: u.Username,
		}
		if err := tx.GetUser(ctx, &pu); err != nil {
			return err
		}
		u.Model = pu.Model
		u.LastLogin = pu.LastLogin
		if u.ControllerAccess == "" {
			u.ControllerAccess = pu.ControllerAccess
		}
		if u.AuditLogAccess == "" {
			u.AuditLogAccess = pu.AuditLogAccess
		}
		// TODO(mhilton) support disabled users.
		if u.DisplayName != "" {
			pu.DisplayName = u.DisplayName
		}
		pu.LastLogin.Time = j.Database.DB.Config.NowFunc()
		pu.LastLogin.Valid = true
		return tx.UpdateUser(ctx, &pu)
	})
	if err != nil {
		return nil, errors.E(op, err)
	}
	return u, nil
}
