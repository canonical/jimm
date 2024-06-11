// Copyright 2020 Canonical Ltd.

package db

import (
	"context"
	"fmt"

	"gorm.io/gorm/clause"

	"github.com/canonical/jimm/internal/dbmodel"
	"github.com/canonical/jimm/internal/errors"
	"github.com/canonical/jimm/internal/servermon"
)

// SetCloudCredential upserts the cloud credential information.
func (d *Database) SetCloudCredential(ctx context.Context, cred *dbmodel.CloudCredential) (err error) {
	const op = errors.Op("db.SetCloudCredential")
	if err := d.ready(); err != nil {
		return errors.E(op, err)
	}

	durationObserver := servermon.DurationObserver(servermon.DBQueryDurationHistogram, string(op))
	defer durationObserver()
	defer servermon.ErrorCounter(servermon.DBQueryErrorCount, &err, string(op))

	if cred.CloudName == "" || cred.OwnerIdentityName == "" || cred.Name == "" {
		return errors.E(op, errors.CodeBadRequest, fmt.Sprintf("invalid cloudcredential tag %q", cred.CloudName+"/"+cred.OwnerIdentityName+"/"+cred.Name))
	}

	db := d.DB.WithContext(ctx)
	if err := db.Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "cloud_name"},
			{Name: "owner_identity_name"},
			{Name: "name"},
		},
		DoUpdates: clause.AssignmentColumns([]string{"auth_type", "label", "attributes_in_vault", "attributes", "valid"}),
	}).Create(&cred).Error; err != nil {
		return errors.E(op, dbError(err))
	}
	return nil
}

// GetCloudCredential returns cloud credential information based on the
// cloud, owner and name.
func (d *Database) GetCloudCredential(ctx context.Context, cred *dbmodel.CloudCredential) (err error) {
	const op = errors.Op("db.GetCloudCredential")
	if err := d.ready(); err != nil {
		return errors.E(op, err)
	}

	durationObserver := servermon.DurationObserver(servermon.DBQueryDurationHistogram, string(op))
	defer durationObserver()
	defer servermon.ErrorCounter(servermon.DBQueryErrorCount, &err, string(op))

	if cred.CloudName == "" || cred.OwnerIdentityName == "" || cred.Name == "" {
		return errors.E(op, errors.CodeNotFound, fmt.Sprintf("cloudcredential %q not found", cred.CloudName+"/"+cred.OwnerIdentityName+"/"+cred.Name))
	}
	db := d.DB.WithContext(ctx)
	db = db.Preload("Cloud")
	db = db.Preload("Models")
	if err := db.Where("cloud_name = ? AND owner_identity_name = ? AND name = ?", cred.CloudName, cred.OwnerIdentityName, cred.Name).First(&cred).Error; err != nil {
		err := dbError(err)
		if errors.ErrorCode(err) == errors.CodeNotFound {
			return errors.E(op, errors.CodeNotFound, fmt.Sprintf("cloudcredential %q not found", cred.CloudName+"/"+cred.OwnerIdentityName+"/"+cred.Name), err)
		}
		return errors.E(op, err)
	}
	return nil
}

// ForEachCloudCredential iterates through all cloud credentials owned by
// the given identity calling the given function with each one. If cloud is
// specified then the cloud-credentials are filtered to only return
// credentials for that cloud.
func (d *Database) ForEachCloudCredential(ctx context.Context, identityName, cloud string, f func(*dbmodel.CloudCredential) error) (err error) {
	const op = errors.Op("db.ForEachCloudCredential")

	if err := d.ready(); err != nil {
		return errors.E(op, err)
	}

	durationObserver := servermon.DurationObserver(servermon.DBQueryDurationHistogram, string(op))
	defer durationObserver()
	defer servermon.ErrorCounter(servermon.DBQueryErrorCount, &err, string(op))

	db := d.DB.WithContext(ctx)
	mdb := db.Model(dbmodel.CloudCredential{})
	if cloud == "" {
		mdb = mdb.Where("owner_identity_name = ?", identityName)
	} else {
		mdb = mdb.Where("cloud_name = ? AND owner_identity_name = ?", cloud, identityName)
	}
	rows, err := mdb.Rows()
	if err != nil {
		return errors.E(op, dbError(err))
	}
	defer rows.Close()
	for rows.Next() {
		var cred dbmodel.CloudCredential
		if err := db.ScanRows(rows, &cred); err != nil {
			return errors.E(op, dbError(err))
		}
		err = d.GetCloudCredential(ctx, &cred)
		if err != nil {
			return err
		}
		if err := f(&cred); err != nil {
			return err
		}
	}
	if err := rows.Err(); err != nil {
		return errors.E(op, dbError(err))
	}
	return nil
}

// DeleteCloudCredential removes the given CloudCredential from the database.
func (d *Database) DeleteCloudCredential(ctx context.Context, cred *dbmodel.CloudCredential) (err error) {
	const op = errors.Op("db.DeleteCloudCredential")

	if err := d.ready(); err != nil {
		return errors.E(op, err)
	}

	durationObserver := servermon.DurationObserver(servermon.DBQueryDurationHistogram, string(op))
	defer durationObserver()
	defer servermon.ErrorCounter(servermon.DBQueryErrorCount, &err, string(op))

	db := d.DB.WithContext(ctx)
	if err := db.Delete(cred).Error; err != nil {
		err = dbError(err)
		return errors.E(op, err)
	}
	return nil
}
