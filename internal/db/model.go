// Copyright 2020 Canonical Ltd.

package db

import (
	"context"

	"gorm.io/gorm"

	"github.com/canonical/jimm/internal/dbmodel"
	"github.com/canonical/jimm/internal/errors"
)

// AddModel stores the model information.
//   - returns an error with code errors.CodeAlreadyExists if
//     model with the same name already exists.
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
	db := d.DB.WithContext(ctx)
	if model.UUID.Valid {
		db = db.Where("uuid = ?", model.UUID.String)
		if model.ControllerID != 0 {
			db = db.Where("controller_id = ?", model.ControllerID)
		}
	} else if model.ID != 0 {
		db = db.Where("id = ?", model.ID)
	} else if model.OwnerUsername != "" && model.Name != "" {
		db = db.Where("owner_username = ? AND name = ?", model.OwnerUsername, model.Name)
	} else if model.ControllerID != 0 {
		// TODO(ales): fix ordering of where fields and handle error to represent what is *actually* required.
		db = db.Where("controller_id = ?", model.ControllerID)
	} else {
		return errors.E(op, "missing id or uuid", errors.CodeBadRequest)
	}

	db = preloadModel("", db)

	if err := db.First(&model).Error; err != nil {
		err = dbError(err)
		if errors.ErrorCode(err) == errors.CodeNotFound {
			return errors.E(op, err, "model not found")
		}
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

	db := d.DB.WithContext(ctx)
	if err := db.Delete(model, model.ID).Error; err != nil {
		return errors.E(op, dbError(err))
	}
	return nil
}

// ForEachModel iterates through every model calling the given function
// for each one. If the given function returns an error the iteration
// will stop immediately and the error will be returned unmodified.
func (d *Database) ForEachModel(ctx context.Context, f func(m *dbmodel.Model) error) error {
	const op = errors.Op("db.ForEachModel")

	if err := d.ready(); err != nil {
		return errors.E(op, err)
	}

	db := d.DB.WithContext(ctx)
	db = preloadModel("", db)
	rows, err := db.Model(&dbmodel.Model{}).Rows()
	if err != nil {
		return errors.E(op, err)
	}
	defer rows.Close()
	for rows.Next() {
		var m dbmodel.Model
		if err := db.ScanRows(rows, &m); err != nil {
			return errors.E(op, err)
		}
		// ScanRows does not use the preloads added on L141, therefore
		// we need to fetch each model to load the associated
		// fields otherwise the only populated fields will be association
		// IDs.
		if err := d.GetModel(ctx, &m); err != nil {
			return errors.E(op, err)
		}
		if err := f(&m); err != nil {
			return err
		}
	}
	if err := rows.Err(); err != nil {
		return errors.E(op, dbError(err))
	}
	return nil
}

// GetModelsByUUID retrieves a list of models where the model UUIDs are in
// the provided modelUUIDs slice.
func (d *Database) GetModelsByUUID(ctx context.Context, modelUUIDs []string) ([]dbmodel.Model, error) {
	const op = errors.Op("db.GetModelsByUUID")

	if err := d.ready(); err != nil {
		return nil, errors.E(op, err)
	}
	var models []dbmodel.Model
	db := d.DB.WithContext(ctx)
	db = preloadModel("", db)
	err := db.Where("uuid IN ?", modelUUIDs).Find(&models).Error
	if err != nil {
		err = dbError(err)
		if errors.ErrorCode(err) == errors.CodeNotFound {
			return nil, errors.E(op, err, "model not found")
		}
		return nil, errors.E(op, dbError(err))
	}
	return models, nil
}

func preloadModel(prefix string, db *gorm.DB) *gorm.DB {
	if len(prefix) > 0 && prefix[len(prefix)-1] != '.' {
		prefix += "."
	}
	db = db.Preload(prefix + "Owner")
	db = db.Preload(prefix + "Controller")
	db = db.Preload(prefix + "CloudRegion").Preload(prefix + "CloudRegion.Cloud")
	db = db.Preload(prefix + "CloudCredential")
	db = db.Preload(prefix + "Offers").Preload(prefix + "Offers.Connections").Preload(prefix + "Offers.Endpoints").Preload(prefix + "Offers.Spaces")
	db = db.Preload(prefix + "Users").Preload(prefix + "Users.User")

	return db
}
