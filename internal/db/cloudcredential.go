// Copyright 2025 Canonical.

package db

import (
	"context"
	"fmt"

	"gorm.io/gorm/clause"

	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/servermon"
)

// SetCloudCredential upserts the cloud credential information.
func (d *Database) SetCloudCredential(ctx context.Context, cred *dbmodel.CloudCredential) (err error) {
	const op = "db.SetCloudCredential"
	if err := d.ready(); err != nil {
		return errors.E(err)
	}

	durationObserver := servermon.DurationObserver(servermon.DBQueryDurationHistogram, op)
	defer durationObserver()
	defer servermon.ErrorCounter(servermon.DBQueryErrorCount, &err, op)

	if cred.CloudName == "" || cred.OwnerIdentityName == "" || cred.Name == "" {
		return errors.E(errors.CodeBadRequest, fmt.Sprintf("invalid cloudcredential tag %q", cred.CloudName+"/"+cred.OwnerIdentityName+"/"+cred.Name))
	}

	db := d.DB.WithContext(ctx)
	if err := db.Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "cloud_name"},
			{Name: "owner_identity_name"},
			{Name: "name"},
		},
		DoUpdates: clause.AssignmentColumns([]string{"auth_type", "label", "valid"}),
	}).Create(&cred).Error; err != nil {
		return errors.E(dbError(err))
	}
	return nil
}

// GetCloudCredential returns cloud credential information based on the
// cloud, owner and name.
func (d *Database) GetCloudCredential(ctx context.Context, cred *dbmodel.CloudCredential) (err error) {
	const op = "db.GetCloudCredential"
	if err := d.ready(); err != nil {
		return errors.E(err)
	}

	durationObserver := servermon.DurationObserver(servermon.DBQueryDurationHistogram, op)
	defer durationObserver()
	defer servermon.ErrorCounter(servermon.DBQueryErrorCount, &err, op)

	if cred.CloudName == "" || cred.OwnerIdentityName == "" || cred.Name == "" {
		return errors.E(errors.CodeNotFound, fmt.Sprintf("cloudcredential %q not found", cred.CloudName+"/"+cred.OwnerIdentityName+"/"+cred.Name))
	}
	db := d.DB.WithContext(ctx)
	db = db.Preload("Cloud")
	db = db.Preload("Models")
	if err := db.Where("cloud_name = ? AND owner_identity_name = ? AND name = ?", cred.CloudName, cred.OwnerIdentityName, cred.Name).First(&cred).Error; err != nil {
		err := dbError(err)
		if errors.ErrorCode(err) == errors.CodeNotFound {
			return errors.E(errors.CodeNotFound, fmt.Sprintf("cloudcredential %q not found", cred.CloudName+"/"+cred.OwnerIdentityName+"/"+cred.Name), err)
		}
		return errors.E(err)
	}
	return nil
}

// ForEachCloudCredential iterates through all cloud credentials owned by
// the given identity calling the given function with each one. If cloud is
// specified then the cloud-credentials are filtered to only return
// credentials for that cloud.
func (d *Database) ForEachCloudCredential(ctx context.Context, identityName, cloud string, f func(*dbmodel.CloudCredential) error) (err error) {
	const op = "db.ForEachCloudCredential"

	if err := d.ready(); err != nil {
		return errors.E(err)
	}

	durationObserver := servermon.DurationObserver(servermon.DBQueryDurationHistogram, op)
	defer durationObserver()
	defer servermon.ErrorCounter(servermon.DBQueryErrorCount, &err, op)

	db := d.DB.WithContext(ctx)
	db = db.Model(dbmodel.CloudCredential{})
	db = db.Preload("Cloud").Preload("Owner").Preload("Models")
	if cloud == "" {
		db = db.Where("owner_identity_name = ?", identityName)
	} else {
		db = db.Where("cloud_name = ? AND owner_identity_name = ?", cloud, identityName)
	}

	var creds []dbmodel.CloudCredential
	if err := db.Find(&creds).Error; err != nil {
		return errors.E(dbError(err))
	}
	for _, c := range creds {
		if err := f(&c); err != nil {
			return err
		}
	}

	return nil
}

// DeleteCloudCredential removes the given CloudCredential from the database.
func (d *Database) DeleteCloudCredential(ctx context.Context, cred *dbmodel.CloudCredential) (err error) {
	const op = "db.DeleteCloudCredential"

	if err := d.ready(); err != nil {
		return errors.E(err)
	}

	durationObserver := servermon.DurationObserver(servermon.DBQueryDurationHistogram, op)
	defer durationObserver()
	defer servermon.ErrorCounter(servermon.DBQueryErrorCount, &err, op)

	db := d.DB.WithContext(ctx)
	if err := db.Delete(cred).Error; err != nil {
		err = dbError(err)
		return errors.E(err)
	}
	return nil
}
