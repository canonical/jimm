// Copyright 2025 Canonical.

package mocks

import (
	"context"
	"time"

	"github.com/canonical/jimm/v3/internal/db"
	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/openfga"
)

// AuditLogManager is an implementation of the jimm.AuditLogManager interface.
type AuditLogManager struct {
	AddAuditLogEntry_ func(ale *dbmodel.AuditLogEntry)
	FindAuditEvents_  func(ctx context.Context, user *openfga.User, filter db.AuditLogFilter) ([]dbmodel.AuditLogEntry, error)
	PurgeLogs_        func(ctx context.Context, user *openfga.User, before time.Time) (int64, error)
}

func (j *AuditLogManager) AddAuditLogEntry(ale *dbmodel.AuditLogEntry) {
	if j.AddAuditLogEntry_ == nil {
		return
	}
}
func (j *AuditLogManager) FindAuditEvents(ctx context.Context, user *openfga.User, filter db.AuditLogFilter) ([]dbmodel.AuditLogEntry, error) {
	if j.FindAuditEvents_ == nil {
		return nil, errors.E(errors.CodeNotImplemented)
	}
	return j.FindAuditEvents_(ctx, user, filter)
}
func (j *AuditLogManager) PurgeLogs(ctx context.Context, user *openfga.User, before time.Time) (int64, error) {
	if j.PurgeLogs_ == nil {
		return 0, errors.E(errors.CodeNotImplemented)
	}
	return j.PurgeLogs_(ctx, user, before)
}
