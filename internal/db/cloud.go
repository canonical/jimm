// Copyright 2020 Canonical Ltd.

package db

import (
	"context"
	"fmt"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/CanonicalLtd/jimm/internal/dbmodel"
	"github.com/CanonicalLtd/jimm/internal/errors"
)

// AddCloud adds the given cloud to the database. AddCloud returns an error
// with a code of CodeAlreadyExists if there is already a cloud with the
// same name.
func (d *Database) AddCloud(ctx context.Context, c *dbmodel.Cloud) error {
	const op = errors.Op("db.AddCloud")
	if err := d.ready(); err != nil {
		return errors.E(op, err)
	}

	db := d.DB.WithContext(ctx)
	if err := db.Create(c).Error; err != nil {
		err := dbError(err)
		if errors.ErrorCode(err) == errors.CodeAlreadyExists {
			return errors.E(op, fmt.Sprintf("cloud %q already exists", c.Name), err)
		}
		return errors.E(op, err)
	}
	return nil
}

// GetCloud fills in the given cloud document based on the cloud name. If
// no cloud is found with the matching name then an error with a code of
// CodeNotFound will be returned.
func (d *Database) GetCloud(ctx context.Context, c *dbmodel.Cloud) error {
	const op = errors.Op("db.GetCloud")
	if err := d.ready(); err != nil {
		return errors.E(op, err)
	}

	db := d.DB.WithContext(ctx)
	db = db.Where("name = ?", c.Name)
	db = preloadCloud("", db)
	if err := db.First(&c).Error; err != nil {
		err := dbError(err)
		if errors.ErrorCode(err) == errors.CodeNotFound {
			return errors.E(op, fmt.Sprintf("cloud %q not found", c.Name), err)
		}
		return errors.E(op, err)
	}
	return nil
}

// GetClouds retrieves all the clouds from the database.
func (d *Database) GetClouds(ctx context.Context) ([]dbmodel.Cloud, error) {
	const op = errors.Op("db.GetClouds")
	if err := d.ready(); err != nil {
		return nil, errors.E(op, err)
	}

	var clouds []dbmodel.Cloud
	db := d.DB.WithContext(ctx)
	db = preloadCloud("", db)
	if err := db.Find(&clouds).Error; err != nil {
		return nil, errors.E(op, err)
	}
	return clouds, nil
}

// SetCloud creates, or updates, the given cloud document. If the cloud
// already exists it will be unaltered except for the addition of new
// regions and users.
func (d *Database) SetCloud(ctx context.Context, c *dbmodel.Cloud) error {
	const op = errors.Op("db.SetCloud")
	if err := d.ready(); err != nil {
		return errors.E(op, err)
	}

	db := d.DB.WithContext(ctx)
	err := db.Transaction(func(tx *gorm.DB) error {
		err := tx.Omit("Regions").Omit("Users").Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "name"}},
			DoNothing: true,
		}).Create(c).Error
		if err != nil {
			return dbError(err)
		}
		// Merge the regions.
		for i := range c.Regions {
			c.Regions[i].CloudName = c.Name
			err := tx.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "cloud_name"}, {Name: "name"}},
				DoNothing: true,
			}).Create(&c.Regions[i]).Error
			if err != nil {
				return dbError(err)
			}
		}

		// Merge the users.
		for i := range c.Users {
			c.Users[i].CloudName = c.Name
			err := tx.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "cloud_name"}, {Name: "username"}},
				DoUpdates: clause.AssignmentColumns([]string{"updated_at", "access"}),
			}).Create(&c.Users[i]).Error
			if err != nil {
				return dbError(err)
			}
		}
		return nil
	})
	if err != nil {
		return errors.E(op, err)
	}
	return nil
}

// UpdateCloud updates the database definition of the cloud to match the
// given cloud. UpdateCloud does not update any user information.
func (d *Database) UpdateCloud(ctx context.Context, c *dbmodel.Cloud) error {
	const op = errors.Op("db.SetCloud")
	if err := d.ready(); err != nil {
		return errors.E(op, err)
	}

	err := d.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(c).Error; err != nil {
			return err
		}
		for _, r := range c.Regions {
			r.CloudName = c.Name
			if err := tx.Save(&r).Error; err != nil {
				return err
			}
			for _, ctl := range r.Controllers {
				ctl.CloudRegionID = r.ID
				if err := tx.Save(&ctl).Error; err != nil {
					return err
				}
			}
		}
		return nil
	})
	if err != nil {
		return errors.E(op, dbError(err))
	}
	return nil
}

func preloadCloud(prefix string, db *gorm.DB) *gorm.DB {
	if len(prefix) > 0 && prefix[len(prefix)-1] != '.' {
		prefix += "."
	}
	db = db.Preload(prefix + "Regions").Preload(prefix + "Regions.Controllers").Preload(prefix + "Regions.Controllers.Controller")
	db = db.Preload(prefix + "Users").Preload(prefix + "Users.User")
	return db
}

// FindRegion finds a region with the given name on a cloud with the given
// provider type.
func (d *Database) FindRegion(ctx context.Context, providerType, name string) (*dbmodel.CloudRegion, error) {
	const op = errors.Op("db.FindRegion")
	if err := d.ready(); err != nil {
		return nil, errors.E(op, err)
	}

	db := d.DB.WithContext(ctx)
	db = db.Joins("Cloud").Preload("Cloud.Users").Preload("Controllers").Preload("Controllers.Controller")
	db = db.Model(dbmodel.CloudRegion{}).Where("cloud.type = ? AND cloud_regions.name = ?", providerType, name)

	var region dbmodel.CloudRegion
	if err := db.First(&region).Error; err != nil {
		return nil, errors.E(op, dbError(err))
	}
	return &region, nil
}

// UpdateUserCloudAccess updates the given UserCloudAccess record. If the
// specified access is changed to "" (no access) then the record is
// removed.
func (d *Database) UpdateUserCloudAccess(ctx context.Context, a *dbmodel.UserCloudAccess) error {
	const op = errors.Op("db.UpdateUserCloudAccess")

	if err := d.ready(); err != nil {
		return errors.E(op, err)
	}

	db := d.DB.WithContext(ctx)
	if a.Access == "" {
		db = db.Delete(a)
	} else {
		db = db.Save(a)
	}
	if db.Error != nil {
		return errors.E(op, dbError(db.Error))
	}
	return nil
}

// DeleteCloud deletes the given cloud.
func (d *Database) DeleteCloud(ctx context.Context, c *dbmodel.Cloud) error {
	const op = errors.Op("db.DeleteCloud")

	if err := d.ready(); err != nil {
		return errors.E(op, err)
	}

	db := d.DB.WithContext(ctx)
	if err := db.Delete(c).Error; err != nil {
		return errors.E(op, dbError(err))
	}
	return nil
}
