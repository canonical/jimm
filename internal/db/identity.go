// Copyright 2020 Canonical Ltd.

package db

import (
	"context"

	"github.com/canonical/jimm/internal/dbmodel"
	"github.com/canonical/jimm/internal/errors"
	"github.com/canonical/jimm/internal/servermon"
)

// GetIdentity loads the details for the identity identified by name. If
// necessary the identity record will be created, in which case the identity will
// have access to no resources and the default add-model access on JIMM.
//
// GetIdentity does not fill out the identity's ApplicationOffers, Clouds,
// CloudCredentials, or Models associations. See GetIdentityApplicationOffers,
// GetIdentityClouds, GetIdentityCloudCredentials, and GetIdentityModels to retrieve
// this information.
//
// GetIdentity returns an error with CodeNotFound if the identity name is invalid.
func (d *Database) GetIdentity(ctx context.Context, u *dbmodel.Identity) (err error) {
	const op = errors.Op("db.GetIdentity")

	if u.Name == "" {
		return errors.E(op, errors.CodeNotFound, `invalid identity name ""`)
	}

	if err := d.ready(); err != nil {
		return errors.E(op, err)
	}

	durationObserver := servermon.DurationObserver(servermon.DBQueryDurationHistogram, string(op))
	defer durationObserver()
	defer servermon.ErrorCounter(servermon.DBQueryErrorCount, &err, string(op))

	db := d.DB.WithContext(ctx)
	if err := db.Where("name = ?", u.Name).FirstOrCreate(&u).Error; err != nil {
		return errors.E(op, err)
	}
	return nil
}

// FetchIdentity loads the details for the identity identified by name. It
// will not create an identity if the identity cannot be found.
//
// FetchIdentity returns an error with CodeNotFound if the identity name is invalid.
func (d *Database) FetchIdentity(ctx context.Context, u *dbmodel.Identity) (err error) {
	const op = errors.Op("db.FetchIdentity")

	if u.Name == "" {
		return errors.E(op, errors.CodeNotFound, `invalid identity name ""`)
	}

	if err := d.ready(); err != nil {
		return errors.E(op, err)
	}

	durationObserver := servermon.DurationObserver(servermon.DBQueryDurationHistogram, string(op))
	defer durationObserver()
	defer servermon.ErrorCounter(servermon.DBQueryErrorCount, &err, string(op))

	db := d.DB.WithContext(ctx)
	if err := db.Where("name = ?", u.Name).First(&u).Error; err != nil {
		return errors.E(op, err)
	}
	return nil
}

// UpdateIdentity updates the given identity record. UpdateIdentity will not store any
// changes to an identity's ApplicationOffers, Clouds, CloudCredentials, or
// Models. These should be updated through the object in question.
//
// UpdateIdentity returns an error with CodeNotFound if the identity name is
// invalid.
func (d *Database) UpdateIdentity(ctx context.Context, u *dbmodel.Identity) (err error) {
	const op = errors.Op("db.UpdateIdentity")
	if err := d.ready(); err != nil {
		return errors.E(op, err)
	}

	durationObserver := servermon.DurationObserver(servermon.DBQueryDurationHistogram, string(op))
	defer durationObserver()
	defer servermon.ErrorCounter(servermon.DBQueryErrorCount, &err, string(op))

	if u.Name == "" {
		return errors.E(op, errors.CodeNotFound, `invalid identity name ""`)
	}

	db := d.DB.WithContext(ctx)
	db = db.Omit("ApplicationOffers").Omit("Clouds").Omit("CloudCredentials").Omit("Models")
	if err := db.Save(u).Error; err != nil {
		return errors.E(op)
	}
	return nil
}

// GetIdentityCloudCredentials fetches identity's cloud credentials for the specified cloud.
func (d *Database) GetIdentityCloudCredentials(ctx context.Context, u *dbmodel.Identity, cloud string) (_ []dbmodel.CloudCredential, err error) {
	const op = errors.Op("db.GetIdentityCloudCredentials")

	if u.Name == "" || cloud == "" {
		return nil, errors.E(op, errors.CodeNotFound, `cloudcredential not found`)
	}

	if err := d.ready(); err != nil {
		return nil, errors.E(op, err)
	}

	durationObserver := servermon.DurationObserver(servermon.DBQueryDurationHistogram, string(op))
	defer durationObserver()
	defer servermon.ErrorCounter(servermon.DBQueryErrorCount, &err, string(op))

	var credentials []dbmodel.CloudCredential
	db := d.DB.WithContext(ctx)
	if err := db.Model(u).Where("cloud_name = ?", cloud).Association("CloudCredentials").Find(&credentials); err != nil {
		return nil, errors.E(op, err)
	}
	return credentials, nil
}
