// Copyright 2024 Canonical.

package db

import (
	"context"

	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/servermon"
)

// AddRole adds a new role.
func (d *Database) AddRole(ctx context.Context, name string) (re *dbmodel.RoleEntry, err error) {
	const op = errors.Op("db.AddRole")
	if err := d.ready(); err != nil {
		return nil, errors.E(op, err)
	}

	durationObserver := servermon.DurationObserver(servermon.DBQueryDurationHistogram, string(op))
	defer durationObserver()
	defer servermon.ErrorCounter(servermon.DBQueryErrorCount, &err, string(op))

	re = &dbmodel.RoleEntry{
		Name: name,
		UUID: newUUID(),
	}

	if err := d.DB.WithContext(ctx).Create(re).Error; err != nil {
		return nil, errors.E(op, dbError(err))
	}
	return re, nil
}

// GetRole populates the provided *dbmodel.RoleEntry based on name or UUID.
func (d *Database) GetRole(ctx context.Context, role *dbmodel.RoleEntry) (err error) {
	const op = errors.Op("db.GetRole")
	if err := d.ready(); err != nil {
		return errors.E(op, err)
	}

	durationObserver := servermon.DurationObserver(servermon.DBQueryDurationHistogram, string(op))
	defer durationObserver()
	defer servermon.ErrorCounter(servermon.DBQueryErrorCount, &err, string(op))

	if role.UUID == "" && role.Name == "" {
		return errors.E(op, "must specify uuid or name")
	}

	db := d.DB.WithContext(ctx)
	if role.ID != 0 {
		db = db.Where("id = ?", role.ID)
	}
	if role.UUID != "" {
		db = db.Where("uuid = ?", role.UUID)
	}
	if role.Name != "" {
		db = db.Where("name = ?", role.Name)
	}
	if err := db.First(&role).Error; err != nil {
		return errors.E(op, dbError(err))
	}
	return nil
}

// UpdateRoleName updates the name of a role identified by name.
func (d *Database) UpdateRoleName(ctx context.Context, oldName, name string) (err error) {
	const op = errors.Op("db.UpdateRole")

	if oldName == "" {
		return errors.E(op, "name must be specified")
	}

	if err := d.ready(); err != nil {
		return errors.E(op, err)
	}

	durationObserver := servermon.DurationObserver(servermon.DBQueryDurationHistogram, string(op))
	defer durationObserver()
	defer servermon.ErrorCounter(servermon.DBQueryErrorCount, &err, string(op))

	model := d.DB.WithContext(ctx).Model(&dbmodel.RoleEntry{})
	model.Where("name = ?", oldName)
	if model.Update("name", name).RowsAffected == 0 {
		return errors.E(op, errors.CodeNotFound, "role not found")
	}

	return nil
}

// RemoveRole removes the role identified by its ID or UUID.
func (d *Database) RemoveRole(ctx context.Context, role *dbmodel.RoleEntry) (err error) {
	const op = errors.Op("db.RemoveRole")

	if role.ID == 0 && role.UUID == "" {
		return errors.E("neither role UUID or ID specified", errors.CodeNotFound)
	}

	if err := d.ready(); err != nil {
		return errors.E(op, err)
	}

	durationObserver := servermon.DurationObserver(servermon.DBQueryDurationHistogram, string(op))
	defer durationObserver()
	defer servermon.ErrorCounter(servermon.DBQueryErrorCount, &err, string(op))

	if err := d.DB.WithContext(ctx).Delete(role).Error; err != nil {
		return errors.E(op, dbError(err))
	}
	return nil
}

// ListRoles returns a paginated list of Roles defined by limit and offset.
// match is used to fuzzy find based on entries' name or uuid using the LIKE operator (ex. LIKE %<match>%).
func (d *Database) ListRoles(ctx context.Context, limit, offset int, match string) (_ []dbmodel.RoleEntry, err error) {
	const op = errors.Op("db.ListRoles")
	if err := d.ready(); err != nil {
		return nil, errors.E(op, err)
	}

	durationObserver := servermon.DurationObserver(servermon.DBQueryDurationHistogram, string(op))
	defer durationObserver()
	defer servermon.ErrorCounter(servermon.DBQueryErrorCount, &err, string(op))

	db := d.DB.WithContext(ctx)
	if match != "" {
		db = db.Where("name LIKE ? OR uuid LIKE ?", "%"+match+"%", "%"+match+"%")
	}
	db = db.Order("name asc")
	db = db.Limit(limit)
	db = db.Offset(offset)
	var Roles []dbmodel.RoleEntry
	if err := db.Find(&Roles).Error; err != nil {
		return nil, errors.E(op, dbError(err))
	}
	return Roles, nil
}

// CountRoles returns a count of the number of Roles that exist.
func (d *Database) CountRoles(ctx context.Context) (count int, err error) {
	const op = errors.Op("db.CountRoles")
	if err := d.ready(); err != nil {
		return 0, errors.E(op, err)
	}
	durationObserver := servermon.DurationObserver(servermon.DBQueryDurationHistogram, string(op))
	defer durationObserver()
	defer servermon.ErrorCounter(servermon.DBQueryErrorCount, &err, string(op))

	var c int64
	var g dbmodel.RoleEntry
	if err := d.DB.WithContext(ctx).Model(g).Count(&c).Error; err != nil {
		return 0, errors.E(op, dbError(err))
	}
	count = int(c)
	return count, nil
}
