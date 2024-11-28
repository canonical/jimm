// Copyright 2024 Canonical.

package db

import (
	"context"

	"github.com/google/uuid"

	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/servermon"
)

var newUUID = uuid.NewString

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

	if group.UUID == "" && group.Name == "" {
		return errors.E(op, "must specify uuid or name")
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

// ListGroups returns a paginated list of groups defined by limit and offset.
// match is used to fuzzy find based on entries' name or uuid using the LIKE operator (ex. LIKE %<match>%).
func (d *Database) ListGroups(ctx context.Context, limit, offset int, match string) (_ []dbmodel.GroupEntry, err error) {
	const op = errors.Op("db.ListGroups")
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
	if limit > 0 {
		db = db.Limit(limit)
	}
	if offset > 0 {
		db = db.Offset(offset)
	}
	var groups []dbmodel.GroupEntry
	if err := db.Find(&groups).Error; err != nil {
		return nil, errors.E(op, dbError(err))
	}
	return groups, nil
}

// UpdateGroupName updates the name of the group identified by its UUID.
func (d *Database) UpdateGroupName(ctx context.Context, uuid, name string) (err error) {
	const op = errors.Op("db.UpdateGroup")

	if uuid == "" {
		return errors.E(op, "uuid must be specified")
	}

	if err := d.ready(); err != nil {
		return errors.E(op, err)
	}

	durationObserver := servermon.DurationObserver(servermon.DBQueryDurationHistogram, string(op))
	defer durationObserver()
	defer servermon.ErrorCounter(servermon.DBQueryErrorCount, &err, string(op))

	model := d.DB.WithContext(ctx).Model(&dbmodel.GroupEntry{})
	model.Where("uuid = ?", uuid)
	if model.Update("name", name).RowsAffected == 0 {
		return errors.E(op, errors.CodeNotFound, "group not found")
	}
	return nil
}

// RemoveGroup removes the group identified by its ID.
func (d *Database) RemoveGroup(ctx context.Context, group *dbmodel.GroupEntry) (err error) {
	const op = errors.Op("db.RemoveGroup")

	if group.ID == 0 && group.UUID == "" {
		return errors.E("neither UUID or ID specified", errors.CodeNotFound)
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
