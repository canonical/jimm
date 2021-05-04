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
	db = db.Preload("CloudRegions").Preload("CloudRegions.CloudRegion").Preload("CloudRegions.CloudRegion.Cloud")
	if err := db.First(&controller).Error; err != nil {
		err = dbError(err)
		if errors.ErrorCode(err) == errors.CodeNotFound {
			return errors.E(op, err, "controller not found")
		}
		return errors.E(op, err)
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
		return errors.E(op, dbError(err))
	}
	return nil
}

// DeleteController removes the specified controller from the database.
func (d *Database) DeleteController(ctx context.Context, controller *dbmodel.Controller) error {
	const op = errors.Op("db.DeleteController")
	if err := d.ready(); err != nil {
		return errors.E(op, err)
	}

	if controller.ID == 0 {
		return errors.E(op, errors.CodeNotFound, `controller not found`)
	}

	db := d.DB.WithContext(ctx)
	if err := db.Delete(controller).Error; err != nil {
		err := dbError(err)
		if errors.ErrorCode(err) == errors.CodeNotFound {
			return errors.E(op, err, "controller not found")
		}
		return errors.E(op, err)
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
	db = db.Preload("CloudRegions").Preload("CloudRegions.CloudRegion").Preload("CloudRegions.CloudRegion.Cloud")
	rows, err := db.Model(&dbmodel.Controller{}).Order("name asc").Rows()
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

// ForEachControllerModel iterates through every model running on the given
// controller calling the given function for each one. If the given
// function returns an error the iteration will stop immediately and the
// error will be returned unmodified.
func (d *Database) ForEachControllerModel(ctx context.Context, ctl *dbmodel.Controller, f func(m *dbmodel.Model) error) error {
	const op = errors.Op("db.ForEachControllerModel")

	if err := d.ready(); err != nil {
		return errors.E(op, err)
	}

	var models []dbmodel.Model
	db := d.DB.WithContext(ctx)
	if err := db.Model(ctl).Association("Models").Find(&models); err != nil {
		return errors.E(op, dbError(err))
	}
	for _, m := range models {
		if err := f(&m); err != nil {
			return err
		}
	}
	return nil
}

// UpsertControllerConfig upserts the controller config.
func (d *Database) UpsertControllerConfig(ctx context.Context, cfg *dbmodel.ControllerConfig) error {
	const op = errors.Op("db.UpsertControllerConfig")

	if err := d.ready(); err != nil {
		return errors.E(op, err)
	}

	if cfg.Name == "" {
		return errors.E(op, errors.CodeBadRequest, `invalid config name ""`)
	}

	db := d.DB.WithContext(ctx)
	if err := db.Save(cfg).Error; err != nil {
		return errors.E(op)
	}
	return nil
}

func (d *Database) GetControllerConfig(ctx context.Context, cfg *dbmodel.ControllerConfig) error {
	const op = errors.Op("db.GetControllerConfig")
	if err := d.ready(); err != nil {
		return errors.E(op, err)
	}

	if cfg.Name == "" {
		return errors.E(op, errors.CodeNotFound, `invalid config name ""`)
	}

	db := d.DB.WithContext(ctx)
	if err := db.Where("name = ?", cfg.Name).First(&cfg).Error; err != nil {
		err := dbError(err)
		if errors.ErrorCode(err) == errors.CodeNotFound {
			return errors.E(op, err, "controller config not found")
		}
		return errors.E(op, err)
	}
	return nil
}
