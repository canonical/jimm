// Copyright 2020 Canonical Ltd.

package db

import (
	"context"

	"gorm.io/gorm"

	"github.com/CanonicalLtd/jimm/internal/dbmodel"
	"github.com/CanonicalLtd/jimm/internal/errors"
)

// AddApplicationOffer stores the application offer information.
func (d *Database) AddApplicationOffer(ctx context.Context, offer *dbmodel.ApplicationOffer) error {
	const op = errors.Op("db.AddApplicationOffer")
	if err := d.ready(); err != nil {
		return errors.E(op, err)
	}
	db := d.DB.WithContext(ctx)

	result := db.Create(offer)
	if result.Error != nil {
		return errors.E(op, dbError(result.Error))
	}
	return nil
}

// UpdateApplicationOffer updates the application offer information.
func (d *Database) UpdateApplicationOffer(ctx context.Context, offer *dbmodel.ApplicationOffer) error {
	const op = errors.Op("db.UpdateApplicationOffer")
	if err := d.ready(); err != nil {
		return errors.E(op, err)
	}
	db := d.DB.WithContext(ctx)

	result := db.Session(&gorm.Session{FullSaveAssociations: true}).Save(offer)
	if result.Error != nil {
		return errors.E(op, dbError(result.Error))
	}

	if err := d.GetApplicationOffer(ctx, offer); err != nil {
		return err
	}

	return nil
}

// GetApplicationOffer returns application offer information based on the
// offer UUID.
func (d *Database) GetApplicationOffer(ctx context.Context, offer *dbmodel.ApplicationOffer) error {
	const op = errors.Op("db.GetApplicationOffer")
	if err := d.ready(); err != nil {
		return errors.E(op, err)
	}
	db := d.DB.WithContext(ctx)

	if offer.UUID != "" {
		db = db.Where("uuid = ?", offer.UUID)
	} else if offer.URL != "" {
		db = db.Where("url = ?", offer.URL)
	} else {
		return errors.E(op, "missing offer UUID or URL")
	}
	result := db.Preload("Application").Preload("Application.Model").Preload("Users").Preload("Users.User").Preload("Endpoints").Preload("Spaces").Preload("Connections").First(&offer)
	if result.Error != nil {
		return errors.E(op, dbError(result.Error))
	}
	return nil
}

// DeleteApplicationOffer deletes the application offer.
func (d *Database) DeleteApplicationOffer(ctx context.Context, offer *dbmodel.ApplicationOffer) error {
	const op = errors.Op("db.DeleteApplicationOffer")
	if err := d.ready(); err != nil {
		return errors.E(op, err)
	}
	db := d.DB.WithContext(ctx)

	result := db.Delete(offer)
	if result.Error != nil {
		return errors.E(op, dbError(result.Error))
	}
	return nil
}
