// Copyright 2020 Canonical Ltd.

package jimm

import (
	"context"

	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v4"

	"github.com/canonical/jimm/internal/db"
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

// GrantAuditLogAccess grants audit log access for the target user.
func (j *JIMM) GrantAuditLogAccess(ctx context.Context, user *dbmodel.User, targetUserTag names.UserTag) error {
	const op = errors.Op("jimm.GrantAuditLogAccess")

	if user.ControllerAccess != "superuser" {
		return errors.E(op, errors.CodeUnauthorized, "unauthorized")
	}
	err := j.Database.Transaction(func(db *db.Database) error {

		targetUser := dbmodel.User{}
		targetUser.SetTag(targetUserTag)
		err := j.Database.GetUser(ctx, &targetUser)
		if err != nil {
			return errors.E(err)
		}
		targetUser.AuditLogAccess = "read"
		err = j.Database.UpdateUser(ctx, &targetUser)
		if err != nil {
			return errors.E(err)
		}

		return nil
	})
	if err != nil {
		return errors.E(op, err)
	}
	return nil
}

// RevokeAuditLogAccess revokes audit log access for the target user.
func (j *JIMM) RevokeAuditLogAccess(ctx context.Context, user *dbmodel.User, targetUserTag names.UserTag) error {
	const op = errors.Op("jimm.RevokeAuditLogAccess")

	if user.ControllerAccess != "superuser" {
		return errors.E(op, errors.CodeUnauthorized, "unauthorized")
	}
	err := j.Database.Transaction(func(db *db.Database) error {

		targetUser := dbmodel.User{}
		targetUser.SetTag(targetUserTag)
		err := j.Database.GetUser(ctx, &targetUser)
		if err != nil {
			return errors.E(err)
		}
		targetUser.AuditLogAccess = ""
		err = j.Database.UpdateUser(ctx, &targetUser)
		if err != nil {
			return errors.E(err)
		}

		return nil
	})
	if err != nil {
		return errors.E(op, err)
	}
	return nil
}
