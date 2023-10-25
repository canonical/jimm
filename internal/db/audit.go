// Copyright 2021 Canonical Ltd.

package db

import (
	"context"
	"time"

	"github.com/canonical/jimm/internal/dbmodel"
	"github.com/canonical/jimm/internal/errors"
)

// AddAuditLogEntry adds a new entry to the audit log.
func (d *Database) AddAuditLogEntry(ctx context.Context, ale *dbmodel.AuditLogEntry) error {
	const op = errors.Op("db.AddAuditLogEntry")
	if err := d.ready(); err != nil {
		return errors.E(op, err)
	}

	if err := d.DB.WithContext(ctx).Create(ale).Error; err != nil {
		return errors.E(op, dbError(err))
	}
	return nil
}

// An AuditLogFilter defines a filter for audit-log entries.
type AuditLogFilter struct {
	// Start defines the earliest time to show audit events for. If
	// this is zero then all audit events that are before the End time
	// are found.
	Start time.Time

	// End defines the latest time to show audit events for. If this is
	// zero then all audit events that are after the Start time are
	// found.
	End time.Time

	// Tag defines the tag on the audit log entry to match, if this is
	// empty all tags are matched.
	Tag string

	// UserTag defines the user-tag on the audit log entry to match, if
	// this is empty all user-tags are matched.
	UserTag string

	// Action defines the action on the audit-log entry to match, if
	// this is empty all actions are matched.
	Action string
}

// ForEachAuditLogEntry iterates through all audit log entries that match
// the given filter calling f for each entry. If f returns an error
// iteration stops immediately and the error is retuned unmodified.
func (d *Database) ForEachAuditLogEntry(ctx context.Context, filter AuditLogFilter, f func(*dbmodel.AuditLogEntry) error) error {
	const op = errors.Op("db.ForEachAuditLogEntry")
	if err := d.ready(); err != nil {
		return errors.E(op, err)
	}

	db := d.DB.WithContext(ctx).Model(&dbmodel.AuditLogEntry{})
	if !filter.Start.IsZero() {
		db = db.Where("time >= ?", filter.Start)
	}
	if !filter.End.IsZero() {
		db = db.Where("time <= ?", filter.End)
	}
	if filter.Tag != "" {
		db = db.Where("tag = ?", filter.Tag)
	}
	if filter.UserTag != "" {
		db = db.Where("user_tag = ?", filter.UserTag)
	}
	if filter.Action != "" {
		db = db.Where("action = ?", filter.Action)
	}
	rows, err := db.Rows()
	if err != nil {
		return errors.E(op, err)
	}
	defer rows.Close()
	for rows.Next() {
		var ale dbmodel.AuditLogEntry
		if err := db.ScanRows(rows, &ale); err != nil {
			return errors.E(op, err)
		}
		if err := f(&ale); err != nil {
			return err
		}
	}
	if rows.Err() != nil {
		return errors.E(op, rows.Err())
	}
	return nil
}
