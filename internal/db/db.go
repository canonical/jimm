// Copyright 2020 Canonical Ltd.

// Package db contains routines to store and retrieve data from a database.
package db

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	"github.com/CanonicalLtd/jimm/internal/dbmodel"
	"github.com/CanonicalLtd/jimm/internal/errors"
)

// A Database provides access to the database model.
type Database struct {
	DB    *gorm.DB
	ready bool
}

// Migrate migrates the configured database to have the structure required
// by the current data model. Unless forced the migration will only be
// performed if there is either no current database, or the major version
// of the current database is the same as the target database. If the
// current database is incompatible then an error with a code of
// errors.CodeServerConfiguration will be returned. If force is requested
// then the migration will be performed no matter what the current version
// is. The force parameter should only be set when the migration is
// initiated by a user request.
func (d *Database) Migrate(ctx context.Context, force bool) error {
	const op = errors.Op("db.Migrate")
	if d == nil || d.DB == nil {
		return errors.E(op, errors.CodeServerConfiguration, "database not configured")
	}
	db := d.DB.WithContext(ctx)
	if err := db.AutoMigrate(&dbmodel.Version{}); err != nil {
		return errors.E(op, errorCode(err), err)
	}
	v := dbmodel.Version{Component: dbmodel.Component}
	if err := db.FirstOrCreate(&v).Error; err != nil {
		return errors.E(op, errorCode(err), err)
	}
	if dbmodel.Major == v.Major && dbmodel.Minor <= v.Minor {
		// The database is already at, or past, our current version.
		// Nothing to do.
		d.ready = true
		return nil
	}
	if v.Major != dbmodel.Major && !force && v.Major != 0 {
		return errors.E(op, errors.CodeServerConfiguration, fmt.Sprintf("database has incompatible version %d.%d", v.Major, v.Minor))
	}

	// The major versions are unchanged, the database can be migrated.
	err := db.AutoMigrate(
		&dbmodel.Application{},
		&dbmodel.ApplicationOffer{},
		&dbmodel.Cloud{},
		&dbmodel.CloudCredential{},
		&dbmodel.CloudRegion{},
		&dbmodel.CloudRegionControllerPriority{},
		&dbmodel.Controller{},
		&dbmodel.Machine{},
		&dbmodel.Model{},
		&dbmodel.Unit{},
		&dbmodel.User{},
		&dbmodel.UserApplicationOfferAccess{},
		&dbmodel.UserCloudAccess{},
		&dbmodel.UserModelAccess{},
	)
	if err != nil {
		return errors.E(op, errorCode(err), err)
	}

	v.Major = dbmodel.Major
	v.Minor = dbmodel.Minor
	if err := db.Save(&v).Error; err != nil {
		return errors.E(op, errorCode(err), err)
	}
	d.ready = true
	return nil
}
