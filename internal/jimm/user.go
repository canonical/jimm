// Copyright 2020 Canonical Ltd.

package jimm

import (
	"context"

	jujuparams "github.com/juju/juju/apiserver/params"

	"github.com/canonical/jimm/internal/dbmodel"
	"github.com/canonical/jimm/internal/errors"
)

// Authenticate processes the given LoginRequest using the configured
// authenticator, it then retrieves the user information from the database.
// If the authenticated user does not yet exist in the database it will be
// created using the values returned from the authenticator as the user's
// details. If the authenticator returns a user with ControllerAccess set
// to "superuser" then the authenticated user will be considered a
// superuser for this session, this will not be persisted.
func (j *JIMM) Authenticate(ctx context.Context, req *jujuparams.LoginRequest) (*dbmodel.User, error) {
	const op = errors.Op("jimm.Authenticate")
	if j == nil || j.Authenticator == nil {
		return nil, errors.E(op, errors.CodeServerConfiguration, "authenticator not configured")
	}

	u, err := j.Authenticator.Authenticate(ctx, req)
	if err != nil {
		return nil, errors.E(op, err)
	}
	isSuperuser := u.ControllerAccess == "superuser"
	u.ControllerAccess = ""

	if err := j.Database.GetUser(ctx, u); err != nil {
		return nil, errors.E(op, err)
	}
	// Update the last-login time.
	u.LastLogin.Time = j.Database.DB.Config.NowFunc()
	u.LastLogin.Valid = true
	if err := j.Database.UpdateUser(ctx, u); err != nil {
		return nil, errors.E(op, err)
	}

	if isSuperuser {
		u.ControllerAccess = "superuser"
	}
	return u, nil
}
