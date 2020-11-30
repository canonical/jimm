// Copyright 2020 Canonical Ltd.

package db

import (
	"context"

	"github.com/CanonicalLtd/jimm/internal/dbmodel"
	"github.com/CanonicalLtd/jimm/internal/errors"
)

// AddController stores the controller information.
func (d *Database) AddController(ctx context.Context, controller *dbmodel.Controller) error {
	const op = errors.Op("db.AddController")
	if err := d.ready(); err != nil {
		return errors.E(op, err)
	}
	db := d.DB.WithContext(ctx)

	if err := db.Create(controller).Error; err != nil {
		return errors.E(op, dbError(err))
	}
	return nil
}

// GetController returns controller information based on the
// controller UUID or name.
func (d *Database) GetController(ctx context.Context, controller *dbmodel.Controller) error {
	const op = errors.Op("db.GetController")
	if err := d.ready(); err != nil {
		return errors.E(op, err)
	}
	db := d.DB.WithContext(ctx)

	if controller.UUID != "" {
		db = db.Where("uuid = ?", controller.UUID)
	}
	if controller.Name != "" {
		db = db.Where("name = ?", controller.Name)
	}

	// TODO (ashipika): think about model preloading - is it needed?
	if err := db.Preload("Models", "is_controller = true").First(&controller).Error; err != nil {
		return errors.E(op, dbError(err))
	}
	return nil
}
