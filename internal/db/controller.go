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

	if err := db.Preload("CloudRegions").First(&controller).Error; err != nil {
		return errors.E(op, dbError(err))
	}
	return nil
}

// UpdateController updates the given controller record. UpdateController will not store any
// changes to a controller's CloudRegions or Models.
func (d *Database) UpdateController(ctx context.Context, controller *dbmodel.Controller) error {
	const op = errors.Op("db.UpdateController")
	if err := d.ready(); err != nil {
		return errors.E(op, err)
	}

	if controller.ID == 0 {
		return errors.E(op, errors.CodeNotFound, `controller not found`)
	}

	db := d.DB.WithContext(ctx)
	db = db.Omit("CloudRegions").Omit("Models")
	if err := db.Save(controller).Error; err != nil {
		return errors.E(op)
	}
	return nil
}

// ForEachController iterates through every controller calling the given function
// for each one. If the given function returns an error the iteration
// will stop immediately and the error will be returned unmodified.
func (d *Database) ForEachController(ctx context.Context, f func(*dbmodel.Controller) error) error {
	const op = errors.Op("db.ForEachController")

	if err := d.ready(); err != nil {
		return errors.E(op, err)
	}

	db := d.DB.WithContext(ctx)
	rows, err := db.Model(&dbmodel.Controller{}).Rows()
	if err != nil {
		return errors.E(op, err)
	}
	defer rows.Close()
	for rows.Next() {
		var controller dbmodel.Controller
		if err := db.ScanRows(rows, &controller); err != nil {
			return errors.E(op, err)
		}
		if err := f(&controller); err != nil {
			return err
		}
	}
	if err := rows.Err(); err != nil {
		return errors.E(op, dbError(err))
	}
	return nil
}
