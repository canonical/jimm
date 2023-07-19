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

	// UserTag defines the user-tag on the audit log entry to match, if
	// this is empty all user-tags are matched.
	UserTag string

	// Model is used to filter the event log to only contain events that
	// were performed against a specific model.
	Model string `json:"model,omitempty"`

	// Method is used to filter the event log to only contain events that
	// called a specific facade method.
	Method string `json:"method,omitempty"`

	// Offset is an offset that will be added when retrieving audit logs.
	// An empty offset is equivalent to zero.
	Offset int `json:"offset,omitempty"`

	// Limit is the maximum number of audit events to return.
	// A value of zero will ignore the limit.
	Limit int `json:"limit,omitempty"`
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
	if filter.UserTag != "" {
		db = db.Where("user_tag = ?", filter.UserTag)
	}
	if filter.Model != "" {
		db = db.Where("model = ?", filter.Model)
	}
	if filter.Method != "" {
		db = db.Where("facade_method = ?", filter.Method)
	}
	db = db.Limit(filter.Limit)
	db = db.Offset(filter.Offset)

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
