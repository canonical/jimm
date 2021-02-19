// Copyright 2020 Canonical Ltd.

package db

import (
	"context"

	"gorm.io/gorm"

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

// GetOption lets you specify which fields are to be populated when
// fetching an object from the database.
type GetOption func(*gorm.DB) *gorm.DB

// AssociatedApplications populates the associated applications.
func AssociatedApplications() GetOption {
	return func(db *gorm.DB) *gorm.DB {
		return db.Preload("Applications")
	}
}

// GetModel returns model information based on the
// model UUID.
func (d *Database) GetModel(ctx context.Context, model *dbmodel.Model, options ...GetOption) error {
	const op = errors.Op("db.GetModel")
	if err := d.ready(); err != nil {
		return errors.E(op, err)
	}
	db := d.DB.WithContext(ctx)
	// TODO (ashipika) consider which fields we need to preload
	// when fetching a model.
	if model.UUID.Valid {
		db = db.Where("uuid = ?", model.UUID.String)
	} else if model.ID != 0 {
		db = db.Where("id = ?", model.ID)
	} else {
		return errors.E(op, "missing id or uuid", errors.CodeBadRequest)
	}

	db = db.Preload("Owner")
	db = db.Preload("Controller")
	db = db.Preload("CloudRegion").Preload("CloudRegion.Cloud")
	db = db.Preload("CloudCredential")
	db = db.Preload("Applications")
	db = db.Preload("Machines")
	db = db.Preload("Users").Preload("Users.User")
	
	for _, option := range options {
		db = option(db)
	}
	
	if err := db.First(&model).Error; err != nil {
		return errors.E(op, dbError(err))
	}
	return nil
}

// GetModelsUsingCredential returns all models that use the specified credentials.
func (d *Database) GetModelsUsingCredential(ctx context.Context, credentialID uint) ([]dbmodel.Model, error) {
	const op = errors.Op("db.GetModelsUsingCredential")
	if err := d.ready(); err != nil {
		return nil, errors.E(op, err)
	}
	db := d.DB.WithContext(ctx)
	var models []dbmodel.Model
	result := db.Where("cloud_credential_id = ?", credentialID).Preload("Controller").Find(&models)
	if result.Error != nil {
		return nil, errors.E(op, dbError(result.Error))
	}
	return models, nil
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

// DeleteModel removes the model information from the database.
func (d *Database) DeleteModel(ctx context.Context, model *dbmodel.Model) error {
	const op = errors.Op("db.DeleteModel")
	if err := d.ready(); err != nil {
		return errors.E(op, err)
	}

	if err := d.DB.Delete(model, model.ID).Error; err != nil {
		return errors.E(op, dbError(err))
	}
	return nil
}
