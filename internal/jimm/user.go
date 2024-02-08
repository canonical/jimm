// Copyright 2020 Canonical Ltd.

package jimm

import (
	"context"

	jujuparams "github.com/juju/juju/rpc/params"

	"github.com/canonical/jimm/internal/db"
	"github.com/canonical/jimm/internal/dbmodel"
	"github.com/canonical/jimm/internal/errors"
	"github.com/canonical/jimm/internal/openfga"
)

// Authenticate processes the given LoginRequest using the configured
// authenticator, it then retrieves the user information from the database.
// If the authenticated user does not yet exist in the database it will be
// created using the values returned from the authenticator as the user's
// details. Finally we check if the user is a administrator of JIMM and set
// the JimmAdmin field if this is true which will persist for the duration
// of the websocket connection.
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
	isJimmAdmin, err := openfga.IsAdministrator(ctx, u, j.ResourceTag())
	if err != nil {
		return nil, errors.E(op, err)
	}
	u.JimmAdmin = isJimmAdmin
	return u, nil
}

// GetOpenFGAUserAndAuthorise returns a valid OpenFGA user, this means:
//
//   - It contains a valid dbmodel user, including their email and juju username
//   - Their last user model has been updated for this connection
//   - The user model is wrapped in an OpenFGA user type
//   - Their admin status is checked and placed on the OpenFGA user type the user's
//     details are wrapped in
func (j *JIMM) GetOpenFGAUserAndAuthorise(ctx context.Context, email string) (*openfga.User, error) {
	const op = errors.Op("jimm.GetOpenFGAUser")

	// Validate the user and get the model
	ut, err := j.OAuthAuthenticationService().GetUserModel(email)
	if err != nil {
		return nil, errors.E(op, err)
	}

	// Setup user model using the tag to populate query fields
	user := &dbmodel.User{
		Username:    ut.Id(),
		DisplayName: ut.Name(),
	}

	// Load the users details
	if err = j.Database.Transaction(func(tx *db.Database) error {
		if err := tx.GetUser(ctx, user); err != nil {
			return err
		}
		// got uuid?
		// no?
		// update

		// TODO(ale8k):
		// This logic of updating the users last login should be done else where
		// and not in the retrieval of the user, ideally a new db method
		// to update login times and tokens. For now, it's ok, but it should be
		// moved.
		user.LastLogin.Time = j.Database.DB.Config.NowFunc()
		user.LastLogin.Valid = true
		// TODO(ale8k): Update access token & refresh tokens for this user
		return tx.UpdateUser(ctx, user)
	}); err != nil {
		return nil, errors.E(op, err)
	}

	// Wrap the user in OpenFGA user for administrator check & ready to place
	// on controllerRoot.user
	ofgaUser := openfga.NewUser(user, j.AuthorizationClient())

	// Check if user is admin
	isJimmAdmin, err := openfga.IsAdministrator(ctx, ofgaUser, j.ResourceTag())
	if err != nil {
		return nil, errors.E(op, err)
	}

	// Set the users admin status for the lifecycle of this WS
	ofgaUser.JimmAdmin = isJimmAdmin

	return ofgaUser, nil
}
