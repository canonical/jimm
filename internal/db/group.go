// Copyright 2021 Canonical Ltd.

package db

import (
	"context"

	"github.com/google/uuid"

	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/servermon"
)

var newUUID = func() string {
	return uuid.NewString()
}

// AddGroup adds a new group.
func (d *Database) AddGroup(ctx context.Context, name string) (ge *dbmodel.GroupEntry, err error) {
	const op = errors.Op("db.AddGroup")
	if err := d.ready(); err != nil {
		return nil, errors.E(op, err)
	}

	durationObserver := servermon.DurationObserver(servermon.DBQueryDurationHistogram, string(op))
	defer durationObserver()
	defer servermon.ErrorCounter(servermon.DBQueryErrorCount, &err, string(op))

	ge = &dbmodel.GroupEntry{
		Name: name,
		UUID: newUUID(),
	}

	if err := d.DB.WithContext(ctx).Create(ge).Error; err != nil {
		return nil, errors.E(op, dbError(err))
	}
	return ge, nil
}

// CountGroups returns a count of the number of groups that exist.
func (d *Database) CountGroups(ctx context.Context) (count int, err error) {
	const op = errors.Op("db.CountGroups")
	if err := d.ready(); err != nil {
		return 0, errors.E(op, err)
	}
	durationObserver := servermon.DurationObserver(servermon.DBQueryDurationHistogram, string(op))
	defer durationObserver()
	defer servermon.ErrorCounter(servermon.DBQueryErrorCount, &err, string(op))

	var c int64
	var g dbmodel.GroupEntry
	if err := d.DB.WithContext(ctx).Model(g).Count(&c).Error; err != nil {
		return 0, errors.E(op, dbError(err))
	}
	count = int(c)
	return count, nil
}

// GetGroup returns a GroupEntry with the specified name.
func (d *Database) GetGroup(ctx context.Context, group *dbmodel.GroupEntry) (err error) {
	const op = errors.Op("db.GetGroup")
	if err := d.ready(); err != nil {
		return errors.E(op, err)
	}

	durationObserver := servermon.DurationObserver(servermon.DBQueryDurationHistogram, string(op))
	defer durationObserver()
	defer servermon.ErrorCounter(servermon.DBQueryErrorCount, &err, string(op))

	db := d.DB.WithContext(ctx)
	if group.ID != 0 {
		db = db.Where("id = ?", group.ID)
	}
	if group.UUID != "" {
		db = db.Where("uuid = ?", group.UUID)
	}
	if group.Name != "" {
		db = db.Where("name = ?", group.Name)
	}
	if err := db.First(&group).Error; err != nil {
		return errors.E(op, dbError(err))
	}
	return nil
}

// ForEachGroup iterates through every group calling the given function
// for each one. If the given function returns an error the iteration
// will stop immediately and the error will be returned unmodified.
func (d *Database) ForEachGroup(ctx context.Context, limit, offset int, f func(*dbmodel.GroupEntry) error) (err error) {
	const op = errors.Op("db.ForEachGroup")
	if err := d.ready(); err != nil {
		return errors.E(op, err)
	}

	durationObserver := servermon.DurationObserver(servermon.DBQueryDurationHistogram, string(op))
	defer durationObserver()
	defer servermon.ErrorCounter(servermon.DBQueryErrorCount, &err, string(op))

	db := d.DB.WithContext(ctx)
	db = db.Order("name asc")
	db = db.Limit(limit)
	db = db.Offset(offset)
	rows, err := db.Model(&dbmodel.GroupEntry{}).Rows()
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
func (d *Database) UpdateGroup(ctx context.Context, group *dbmodel.GroupEntry) (err error) {
	const op = errors.Op("db.UpdateGroup")

	if group.ID == 0 {
		return errors.E(errors.CodeNotFound)
	}
	if group.UUID == "" {
		return errors.E("group uuid not specified", errors.CodeNotFound)
	}

	if err := d.ready(); err != nil {
		return errors.E(op, err)
	}

	durationObserver := servermon.DurationObserver(servermon.DBQueryDurationHistogram, string(op))
	defer durationObserver()
	defer servermon.ErrorCounter(servermon.DBQueryErrorCount, &err, string(op))

	if err := d.DB.WithContext(ctx).Save(group).Error; err != nil {
		return errors.E(op, dbError(err))
	}
	return nil
}

// RemoveGroup removes the group identified by its ID.
func (d *Database) RemoveGroup(ctx context.Context, group *dbmodel.GroupEntry) (err error) {
	const op = errors.Op("db.RemoveGroup")

	if group.ID == 0 {
		return errors.E(errors.CodeNotFound)
	}
	if group.UUID == "" {
		return errors.E(errors.CodeNotFound)
	}

	if err := d.ready(); err != nil {
		return errors.E(op, err)
	}

	durationObserver := servermon.DurationObserver(servermon.DBQueryDurationHistogram, string(op))
	defer durationObserver()
	defer servermon.ErrorCounter(servermon.DBQueryErrorCount, &err, string(op))

	if err := d.DB.WithContext(ctx).Delete(group).Error; err != nil {
		return errors.E(op, dbError(err))
	}
	return nil
}
