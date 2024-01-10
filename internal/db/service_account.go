// Copyright 2024 Canonical Ltd.

package db

import (
	"context"

	"github.com/canonical/jimm/internal/dbmodel"
	"github.com/canonical/jimm/internal/errors"
)

// GetServiceAccount loads the details for the service account identified by
// ClientID. If necessary, the service account record will be created.
//
// GetServiceAccount does not fill out the service account's CloudCredentials.
// Use GetServiceAccountCloudCredentials to retrieve this information.
//
// GetServiceAccount returns an error with CodeNotFound if the service account's
// ClientID is invalid.
func (d *Database) GetServiceAccount(ctx context.Context, sa *dbmodel.ServiceAccount) error {
	const op = errors.Op("db.GetServiceAccount")
	if err := d.ready(); err != nil {
		return errors.E(op, err)
	}

	if sa.ClientID == "" {
		return errors.E(op, errors.CodeNotFound, `invalid client id ""`)
	}

	db := d.DB.WithContext(ctx)
	if err := db.Where("client_id = ?", sa.ClientID).FirstOrCreate(&sa).Error; err != nil {
		return errors.E(op, err)
	}
	return nil
}

// FetchServiceAccount loads the details for the service account identified by
// ClientID. It will not create a service account if it was not found.
//
// GetServiceAccount does not fill out the service account's CloudCredentials.
// Use GetServiceAccountCloudCredentials to retrieve this information.
//
// FetchServiceAccount returns an error with CodeNotFound if the ClientID is invalid.
func (d *Database) FetchServiceAccount(ctx context.Context, sa *dbmodel.ServiceAccount) error {
	const op = errors.Op("db.FetchServiceAccount")
	if err := d.ready(); err != nil {
		return errors.E(op, err)
	}

	if sa.ClientID == "" {
		return errors.E(op, errors.CodeNotFound, `invalid client id ""`)
	}

	db := d.DB.WithContext(ctx)
	if err := db.Where("client_id = ?", sa.ClientID).First(&sa).Error; err != nil {
		return errors.E(op, err)
	}
	return nil
}

// UpdateServiceAccount updates the given service account record.
// UpdateServiceAccount will not store any changes to a service account's
// CloudCredentials.
//
// UpdateServiceAccount returns an error with CodeNotFound if the ClientID is
// invalid.
func (d *Database) UpdateServiceAccount(ctx context.Context, sa *dbmodel.ServiceAccount) error {
	const op = errors.Op("db.UpdateServiceAccount")
	if err := d.ready(); err != nil {
		return errors.E(op, err)
	}

	if sa.ClientID == "" {
		return errors.E(op, errors.CodeNotFound, `invalid client id ""`)
	}

	db := d.DB.WithContext(ctx)
	db = db.Omit("CloudCredentials")
	if err := db.Save(sa).Error; err != nil {
		return errors.E(op)
	}
	return nil
}

// GetServiceAccountCloudCredentials fetches service account cloud credentials for the specified cloud.
func (d *Database) GetServiceAccountCloudCredentials(ctx context.Context, sa *dbmodel.ServiceAccount, cloud string) ([]dbmodel.CloudCredential, error) {
	const op = errors.Op("db.GetServiceAccountCloudCredentials")
	if err := d.ready(); err != nil {
		return nil, errors.E(op, err)
	}

	if sa.ClientID == "" || cloud == "" {
		return nil, errors.E(op, errors.CodeNotFound, `cloudcredential not found`)
	}

	var credentials []dbmodel.CloudCredential
	db := d.DB.WithContext(ctx)
	if err := db.Model(sa).Where("cloud_name = ?", cloud).Association("CloudCredentials").Find(&credentials); err != nil {
		return nil, errors.E(op, err)
	}
	return credentials, nil
}
