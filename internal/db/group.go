// Copyright 2021 Canonical Ltd.

package db

import (
	"context"

	"github.com/CanonicalLtd/jimm/internal/dbmodel"
	"github.com/CanonicalLtd/jimm/internal/errors"
)

// AddGroup adds a new group.
func (d *Database) AddGroup(ctx context.Context, name string) error {
	const op = errors.Op("db.AddGroup")
	if err := d.ready(); err != nil {
		return errors.E(op, err)
	}
	ge := dbmodel.GroupEntry{
		Name: name,
	}

	if err := d.DB.WithContext(ctx).Create(&ge).Error; err != nil {
		return errors.E(op, dbError(err))
	}
	return nil
}

// GetGroup returns a GroupEntry with the specified name.
func (d *Database) GetGroup(ctx context.Context, name string) (*dbmodel.GroupEntry, error) {
	const op = errors.Op("db.GetGroup")
	if err := d.ready(); err != nil {
		return nil, errors.E(op, err)
	}
	ge := dbmodel.GroupEntry{
		Name: name,
	}

	if err := d.DB.WithContext(ctx).First(&ge).Error; err != nil {
		return nil, errors.E(op, dbError(err))
	}
	return &ge, nil
}

// ForEachGroup iterates through every group calling the given function
// for each one. If the given function returns an error the iteration
// will stop immediately and the error will be returned unmodified.
func (d *Database) ForEachGroup(ctx context.Context, f func(*dbmodel.GroupEntry) error) error {
	const op = errors.Op("db.ForEachGroup")
	if err := d.ready(); err != nil {
		return errors.E(op, err)
	}
	db := d.DB.WithContext(ctx)
	rows, err := db.Model(&dbmodel.GroupEntry{}).Order("name asc").Rows()
	if err != nil {
		return errors.E(op, err)
	}
	defer rows.Close()
	for rows.Next() {
		var group dbmodel.GroupEntry
		if err := db.ScanRows(rows, &group); err != nil {
			return errors.E(op, err)
		}
		if err := f(&group); err != nil {
			return err
		}
	}
	if err := rows.Err(); err != nil {
		return errors.E(op, dbError(err))
	}
	return nil
}

// UpdateGroup updates the group identified by its ID.
func (d *Database) UpdateGroup(ctx context.Context, group *dbmodel.GroupEntry) error {
	const op = errors.Op("db.UpdateGroup")
	if err := d.ready(); err != nil {
		return errors.E(op, err)
	}
	if group.ID == 0 {
		return errors.E(errors.CodeNotFound)
	}
	if err := d.DB.WithContext(ctx).Save(group).Error; err != nil {
		return errors.E(op, dbError(err))
	}
	return nil
}

// RemoveGroup removes the group identified by its ID.
func (d *Database) RemoveGroup(ctx context.Context, group *dbmodel.GroupEntry) error {
	const op = errors.Op("db.RemoveGroup")
	if err := d.ready(); err != nil {
		return errors.E(op, err)
	}
	if group.ID == 0 {
		return errors.E(errors.CodeNotFound)
	}
	if err := d.DB.WithContext(ctx).Delete(group).Error; err != nil {
		return errors.E(op, dbError(err))
	}
	return nil
}
