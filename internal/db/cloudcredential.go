// Copyright 2020 Canonical Ltd.

package db

import (
	"context"
	"fmt"

	"github.com/CanonicalLtd/jimm/internal/dbmodel"
	"github.com/CanonicalLtd/jimm/internal/errors"
	"gorm.io/gorm/clause"
)

// SetCloudCredential upserts the cloud credential information.
func (d *Database) SetCloudCredential(ctx context.Context, cred *dbmodel.CloudCredential) error {
	const op = errors.Op("db.SetCloudCredential")
	if err := d.ready(); err != nil {
		return errors.E(op, err)
	}

	if cred.CloudName == "" || cred.OwnerID == "" || cred.Name == "" {
		return errors.E(op, errors.CodeBadRequest, fmt.Sprintf("invalid cloudcredential tag %q", cred.CloudName+"/"+cred.OwnerID+"/"+cred.Name))
	}

	db := d.DB.WithContext(ctx)
	if err := db.Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "cloud_name"},
			{Name: "owner_id"},
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
func (d *Database) GetCloudCredential(ctx context.Context, cred *dbmodel.CloudCredential) error {
	const op = errors.Op("db.GetCloudCredential")
	if err := d.ready(); err != nil {
		return errors.E(op, err)
	}
	if cred.CloudName == "" || cred.OwnerID == "" || cred.Name == "" {
		return errors.E(op, errors.CodeNotFound, fmt.Sprintf("cloudcredential %q not found", cred.CloudName+"/"+cred.OwnerID+"/"+cred.Name))
	}
	db := d.DB.WithContext(ctx)
	if err := db.Where("cloud_name = ? AND owner_id = ? AND name = ?", cred.CloudName, cred.OwnerID, cred.Name).First(&cred).Error; err != nil {
		err := dbError(err)
		if errors.ErrorCode(err) == errors.CodeNotFound {
			return errors.E(op, errors.CodeNotFound, fmt.Sprintf("cloudcredential %q not found", cred.CloudName+"/"+cred.OwnerID+"/"+cred.Name), err)
		}
		return errors.E(op, err)
	}
	return nil
}
