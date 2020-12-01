// Copyright 2020 Canonical Ltd.

package db

import (
	"context"

	"github.com/CanonicalLtd/jimm/internal/dbmodel"
	"github.com/CanonicalLtd/jimm/internal/errors"
)

// AddModel stores the model information.
// - returns an error with code errors.CodeAlreadyExists if
//   model with the same name already exists.
func (d *Database) AddModel(ctx context.Context, model *dbmodel.Model) error {
	const op = errors.Op("db.AddModel")
	if err := d.ready(); err != nil {
		return errors.E(op, err)
	}
	db := d.DB.WithContext(ctx)

	if err := db.Create(model).Error; err != nil {
		return errors.E(op, dbError(err))
	}
	return nil
}

// GetModel returns model information based on the
// model UUID.
func (d *Database) GetModel(ctx context.Context, model *dbmodel.Model) error {
	const op = errors.Op("db.GetModel")
	if err := d.ready(); err != nil {
		return errors.E(op, err)
	}
	if model.UUID.String == "" {
		return errors.E(op, errors.CodeNotFound, `invalid uuid ""`)
	}
	db := d.DB.WithContext(ctx)
	// TODO (ashipika) consider which fields we need to preload
	// when fetching a model.
	if err := db.Where("uuid = ?", model.UUID.String).Preload("Users").First(&model).Error; err != nil {
		return errors.E(op, dbError(err))
	}
	return nil
}

// UpdateModel updates the model information.
func (d *Database) UpdateModel(ctx context.Context, model *dbmodel.Model) error {
	const op = errors.Op("db.UpdateModel")
	if err := d.ready(); err != nil {
		return errors.E(op, err)
	}
	db := d.DB.WithContext(ctx)
	if err := db.Save(model).Error; err != nil {
		return errors.E(op, dbError(err))
	}
	return nil
}
