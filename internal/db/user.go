// Copyright 2020 Canonical Ltd.

package db

import (
	"context"

	"github.com/canonical/jimm/internal/dbmodel"
	"github.com/canonical/jimm/internal/errors"
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

// GetUserClouds gets all the UserCloudAccess records associated with the
// given user.
func (d *Database) GetUserClouds(ctx context.Context, u *dbmodel.User) ([]dbmodel.UserCloudAccess, error) {
	const op = errors.Op("db.GetUserClouds")
	if err := d.ready(); err != nil {
		return nil, errors.E(op, err)
	}

	var ucas []dbmodel.UserCloudAccess
	db := d.DB.WithContext(ctx)
	db = preloadCloud("Cloud", db)
	if err := db.Model(u).Association("Clouds").Find(&ucas); err != nil {
		return nil, errors.E(op, err)
	}
	return ucas, nil
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

// GetUserCloudCredentials fetches user cloud credentials for the specified cloud.
func (d *Database) GetUserCloudCredentials(ctx context.Context, u *dbmodel.User, cloud string) ([]dbmodel.CloudCredential, error) {
	const op = errors.Op("db.GetUserCloudCredentials")
	if err := d.ready(); err != nil {
		return nil, errors.E(op, err)
	}

	if u.Username == "" || cloud == "" {
		return nil, errors.E(op, errors.CodeNotFound, `cloudcredential not found`)
	}

	var credentials []dbmodel.CloudCredential
	db := d.DB.WithContext(ctx)
	if err := db.Model(u).Where("cloud_name = ?", cloud).Association("CloudCredentials").Find(&credentials); err != nil {
		return nil, errors.E(op, err)
	}
	return credentials, nil
}

// GetUserModels retrives all the models that the given user has been
// granted access to.
func (d *Database) GetUserModels(ctx context.Context, u *dbmodel.User) ([]dbmodel.UserModelAccess, error) {
	const op = errors.Op("db.GetUserModels")
	if err := d.ready(); err != nil {
		return nil, errors.E(op, err)
	}

	var umas []dbmodel.UserModelAccess
	db := d.DB.WithContext(ctx)
	db = preloadModel("Model_", db)
	if err := db.Model(u).Association("Models").Find(&umas); err != nil {
		return nil, errors.E(op, err)
	}
	return umas, nil
}
