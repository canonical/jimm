// Copyright 2020 Canonical Ltd.

package jimm

import (
	"context"

	"github.com/canonical/jimm/internal/common/pagination"
	"github.com/canonical/jimm/internal/db"
	"github.com/canonical/jimm/internal/dbmodel"
	"github.com/canonical/jimm/internal/errors"
	"github.com/canonical/jimm/internal/openfga"
)

// GetOpenFGAUserAndAuthorise returns a valid OpenFGA user, authorising
// them as an admin of JIMM if a tuple exists for this user.
func (j *JIMM) GetOpenFGAUserAndAuthorise(ctx context.Context, emailOrSvcAccId string) (*openfga.User, error) {
	const op = errors.Op("jimm.GetOpenFGAUserAndAuthorise")

	// TODO(ale8k): Name is email for NOW until we add email field
	// and map emails/usernames to a uuid for the user. Then, queries should be
	// queried upon by uuid, not username.
	// Setup identity model using the tag to populate query fields
	user, err := dbmodel.NewIdentity(emailOrSvcAccId)
	if err != nil {
		return nil, errors.E(op, err)
	}

	// Load the users details
	if err := j.Database.Transaction(func(tx *db.Database) error {
		if err := tx.GetIdentity(ctx, user); err != nil {
			return err
		}

		// TODO(ale8k):
		// This logic of updating the users last login should be done else where
		// and not in the retrieval of the user, ideally a new db method
		// to update login times and tokens. For now, it's ok, but it should be
		// moved.
		user.LastLogin.Time = j.Database.DB.Config.NowFunc()
		user.LastLogin.Valid = true

		return tx.UpdateIdentity(ctx, user)
	}); err != nil {
		return nil, errors.E(op, err)
	}

	// Wrap the user in OpenFGA user for administrator check & ready to place
	// on controllerRoot.user
	ofgaUser := openfga.NewUser(user, j.OpenFGAClient)

	// Check if user is admin
	isJimmAdmin, err := openfga.IsAdministrator(ctx, ofgaUser, j.ResourceTag())
	if err != nil {
		return nil, errors.E(op, err)
	}

	// Set the users admin status for the lifecycle of this WS
	ofgaUser.JimmAdmin = isJimmAdmin

	return ofgaUser, nil
}

// GetUser fetches the user specified by the username and returns
// an openfga User that can be used to verify user's permissions.
func (j *JIMM) GetUser(ctx context.Context, username string) (*openfga.User, error) {
	const op = errors.Op("jimm.GetUser")

	user, err := dbmodel.NewIdentity(username)
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

// FetchUser fetches the user specified by the username and returns the user if it is found.
// Or error "record not found".
func (j *JIMM) FetchUser(ctx context.Context, username string) (*openfga.User, error) {
	const op = errors.Op("jimm.GetUser")

	uEntity, err := dbmodel.NewIdentity(username)
	if err != nil {
		return nil, errors.E(op, err)
	}

	if err := j.Database.FetchIdentity(ctx, uEntity); err != nil {
		return nil, err
	}
	u := openfga.NewUser(uEntity, j.OpenFGAClient)

	return u, nil
}

// ListUsers lists a page of users in our database and parse them into openfga entities.
func (j *JIMM) ListUsers(ctx context.Context, user *openfga.User, filter pagination.LimitOffsetPagination) ([]openfga.User, error) {
	const op = errors.Op("jimm.ListUsers")

	if !user.JimmAdmin {
		return nil, errors.E(op, errors.CodeUnauthorized, "unauthorized")
	}

	var users []openfga.User
	err := j.Database.ForEachIdentity(ctx, filter.Limit(), filter.Offset(), func(ge *dbmodel.Identity) error {
		u := openfga.NewUser(ge, j.OpenFGAClient)
		users = append(users, *u)
		return nil
	})
	if err != nil {
		return nil, errors.E(op, err)
	}
	return users, nil
}

// CountUsers returns the count of all the users in our database.
func (j *JIMM) CountUsers(ctx context.Context, user *openfga.User) (int, error) {
	const op = errors.Op("jimm.ListUsers")

	if !user.JimmAdmin {
		return 0, errors.E(op, errors.CodeUnauthorized, "unauthorized")
	}

	count, err := j.Database.CountIdentities(ctx)
	if err != nil {
		return 0, errors.E(op, err)
	}
	return count, nil
}
