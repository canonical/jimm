// Copyright 2020 Canonical Ltd.

package db

import (
	"context"
	"fmt"

	"gorm.io/gorm/clause"

	"github.com/canonical/jimm/internal/dbmodel"
	"github.com/canonical/jimm/internal/errors"
)

// SetCloudCredential upserts the cloud credential information.
func (d *Database) SetCloudCredential(ctx context.Context, cred *dbmodel.CloudCredential) error {
	const op = errors.Op("db.SetCloudCredential")
	if err := d.ready(); err != nil {
		return errors.E(op, err)
	}

	if cred.CloudName == "" || cred.Name == "" {
		return errors.E(op, errors.CodeBadRequest, fmt.Sprintf("invalid cloudcredential tag %q", cred.CloudName+"/"+cloudCredentialOwnerString(cred)+"/"+cred.Name))
	}

	// Note that cred.OwnerUsername and cred.OwnerClientID are mutually exclusive.
	hasNoOwner := cred.OwnerUsername == nil && cred.OwnerClientID == nil
	if hasNoOwner {
		return errors.E(op, errors.CodeBadRequest, fmt.Sprintf("invalid cloudcredential tag; no owner: %q", cred.CloudName+"/?/"+cred.Name))
	}

	hasMultipleOwners := cred.OwnerUsername != nil && cred.OwnerClientID != nil
	if hasMultipleOwners {
		return errors.E(op, errors.CodeBadRequest, fmt.Sprintf("invalid cloudcredential tag; ; multiple owners: %q and %q", *cred.OwnerUsername, *cred.OwnerClientID))
	}

	var onConflictColumns []clause.Column
	if cred.OwnerUsername != nil {
		onConflictColumns = []clause.Column{
			{Name: "cloud_name"},
			{Name: "owner_username"},
			{Name: "name"},
		}
	} else {
		onConflictColumns = []clause.Column{
			{Name: "cloud_name"},
			{Name: "owner_client_id"},
			{Name: "name"},
		}
	}

	db := d.DB.WithContext(ctx)
	if err := db.Clauses(clause.OnConflict{
		Columns:   onConflictColumns,
		DoUpdates: clause.AssignmentColumns([]string{"auth_type", "label", "attributes_in_vault", "attributes", "valid"}),
	}).Create(&cred).Error; err != nil {
		return errors.E(op, dbError(err))
	}
	return nil
}

// GetCloudCredential returns cloud credential information based on the
// cloud, owner and name.
func (d *Database) GetCloudCredential(ctx context.Context, cred *dbmodel.CloudCredential) error {
	const op = errors.Op("db.GetCloudCredential")
	if err := d.ready(); err != nil {
		return errors.E(op, err)
	}

	if cred.CloudName == "" || cred.Name == "" {
		return errors.E(op, errors.CodeNotFound, fmt.Sprintf("cloudcredential %q not found", cred.CloudName+"/"+cloudCredentialOwnerString(cred)+"/"+cred.Name))
	}

	hasNoOwner := cred.OwnerUsername == nil && cred.OwnerClientID == nil
	if hasNoOwner {
		return errors.E(op, errors.CodeBadRequest, fmt.Sprintf("cloudcredential requires owner: %q", cred.CloudName+"/?/"+cred.Name))
	}

	hasMultipleOwners := cred.OwnerUsername != nil && cred.OwnerClientID != nil
	if hasMultipleOwners {
		return errors.E(op, errors.CodeBadRequest, fmt.Sprintf("cloudcredential must have only one owner: %q or %q", *cred.OwnerUsername, *cred.OwnerClientID))
	}

	db := d.DB.WithContext(ctx)
	db = db.Preload("Cloud")
	db = db.Preload("Models")

	var err error
	if cred.OwnerUsername != nil {
		err = db.Where("cloud_name = ? AND owner_username = ? AND name = ?", cred.CloudName, *cred.OwnerUsername, cred.Name).First(&cred).Error
	} else {
		err = db.Where("cloud_name = ? AND owner_client_id = ? AND name = ?", cred.CloudName, *cred.OwnerClientID, cred.Name).First(&cred).Error
	}

	if err != nil {
		err := dbError(err)
		if errors.ErrorCode(err) == errors.CodeNotFound {
			return errors.E(op, errors.CodeNotFound, fmt.Sprintf("cloudcredential %q not found", cred.CloudName+"/"+cloudCredentialOwnerString(cred)+"/"+cred.Name), err)
		}
		return errors.E(op, err)
	}
	return nil
}

// cloudCredentialOwnerString returns a string representation of a
// cloud-credential's owner(s).
// Note that this method is to avoid duplication and is only intended to be used
// for error message formatting.
func cloudCredentialOwnerString(cred *dbmodel.CloudCredential) string {
	if cred.OwnerUsername == nil && cred.OwnerClientID == nil {
		return "?"
	} else if cred.OwnerUsername != nil && cred.OwnerClientID != nil {
		return "(" + *cred.OwnerUsername + "|" + *cred.OwnerClientID + ")"
	} else if cred.OwnerUsername != nil {
		return *cred.OwnerUsername
	}
	return *cred.OwnerClientID
}

// ForEachCloudCredential iterates through all cloud credentials owned by
// the given user calling the given function with each one. If cloud is
// specified then the cloud-credentials are filtered to only return
// credentials for that cloud.
func (d *Database) ForEachCloudCredential(ctx context.Context, username, cloud string, f func(*dbmodel.CloudCredential) error) error {
	const op = errors.Op("db.ForEachCloudCredential")

	// TODO (babakks): this method queries for User-owned credentials. We might
	// need to update this, or add another method to support credentials owned
	// by service accounts.

	if err := d.ready(); err != nil {
		return errors.E(op, err)
	}

	db := d.DB.WithContext(ctx)
	mdb := db.Model(dbmodel.CloudCredential{})
	if cloud == "" {
		mdb = mdb.Where("owner_username = ?", username)
	} else {
		mdb = mdb.Where("cloud_name = ? AND owner_username = ?", cloud, username)
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
func (d *Database) DeleteCloudCredential(ctx context.Context, cred *dbmodel.CloudCredential) error {
	const op = errors.Op("db.DeleteCloudCredential")

	db := d.DB.WithContext(ctx)
	if err := db.Delete(cred).Error; err != nil {
		err = dbError(err)
		return errors.E(op, err)
	}
	return nil
}
