// Copyright 2021 Canonical Ltd.

package db

import (
	"time"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery/dbrootkeystore"
	"gorm.io/gorm"

	"github.com/canonical/jimm/internal/dbmodel"
	"github.com/canonical/jimm/internal/errors"
)

// GetKey implements Backing.GetKey.
func (d *Database) GetKey(id []byte) (dbrootkeystore.RootKey, error) {
	const op = errors.Op("db.FindLatestKey")

	if d.DB == nil {
		return dbrootkeystore.RootKey{}, bakery.ErrNotFound
	}
	rk := dbmodel.RootKey{
		ID: id,
	}
	if err := d.DB.First(&rk).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return dbrootkeystore.RootKey{}, bakery.ErrNotFound
		}
		return dbrootkeystore.RootKey{}, errors.E(op, err)
	}
	return dbrootkeystore.RootKey{
		Id:      rk.ID,
		Created: rk.CreatedAt,
		Expires: rk.Expires,
		RootKey: rk.RootKey,
	}, nil
}

// FindLatestKey implements Backing.FindLatestKey.
func (d *Database) FindLatestKey(createdAfter, expiresAfter, expiresBefore time.Time) (dbrootkeystore.RootKey, error) {
	const op = errors.Op("db.FindLatestKey")

	if d.DB == nil {
		return dbrootkeystore.RootKey{}, nil
	}
	db := d.DB.Where("created_at > ?", createdAfter)
	db = db.Where("expires BETWEEN ? AND ?", expiresAfter, expiresBefore)
	db = db.Order("created_at DESC")
	var rk dbmodel.RootKey
	if err := db.First(&rk).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return dbrootkeystore.RootKey{}, nil
		}
		return dbrootkeystore.RootKey{}, errors.E(op, dbError(err))
	}
	return dbrootkeystore.RootKey{
		Id:      rk.ID,
		Created: rk.CreatedAt,
		Expires: rk.Expires,
		RootKey: rk.RootKey,
	}, nil
}

// InsertKey implements Backing.InsertKey.
func (d *Database) InsertKey(key dbrootkeystore.RootKey) error {
	const op = errors.Op("db.InsertKey")

	if d.DB == nil {
		return errors.E(op, errors.CodeServerConfiguration, "database not configured")
	}
	rk := dbmodel.RootKey{
		ID:        key.Id,
		CreatedAt: key.Created,
		Expires:   key.Expires,
		RootKey:   key.RootKey,
	}
	if err := d.DB.Create(&rk).Error; err != nil {
		return errors.E(op, dbError(err))
	}
	return nil
}
