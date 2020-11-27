// Copyright 2020 Canonical Ltd.

package db

import (
	"context"

	"github.com/CanonicalLtd/jimm/internal/dbmodel"
	"github.com/CanonicalLtd/jimm/internal/errors"
)

// GetUser loads the user details for the user identified by username. If
// necessary the user record will be created, in which case the user will
// have access to no resources and the default add-model access on JIMM.
//
// GetUser does not fill out the user's ApplicationOffers, Clouds,
// CloudCredentials, or Models associations. See GetUserApplicationOffers,
// GetUserClouds, GetUserCloudCredentials, and GetUserModels to retrieve
// this information.
//
// GetUser returns an error with CodeNotFound if the username is invalid.
func (d *Database) GetUser(ctx context.Context, u *dbmodel.User) error {
	const op = errors.Op("db.GetUser")
	if err := d.ready(); err != nil {
		return errors.E(op, err)
	}

	if u.Username == "" {
		return errors.E(op, errors.CodeNotFound, `invalid username ""`)
	}

	db := d.DB.WithContext(ctx)
	if err := db.Where("username = ?", u.Username).FirstOrCreate(&u).Error; err != nil {
		return errors.E(op, err)
	}
	return nil
}

// UpdateUser updates the given user record. UpdateUser will not store any
// changes to a user's ApplicationOffers, Clouds, CloudCredentials, or
// Models. These should be updated through the object in question.
//
// UpdateUser returns an error with CodeNotFound if the username is
// invalid.
func (d *Database) UpdateUser(ctx context.Context, u *dbmodel.User) error {
	const op = errors.Op("db.UpdateUser")
	if err := d.ready(); err != nil {
		return errors.E(op, err)
	}

	if u.Username == "" {
		return errors.E(op, errors.CodeNotFound, `invalid username ""`)
	}

	db := d.DB.WithContext(ctx)
	db = db.Omit("ApplicationOffers").Omit("Clouds").Omit("CloudCredentials").Omit("Models")
	if err := db.Save(u).Error; err != nil {
		return errors.E(op)
	}
	return nil
}
