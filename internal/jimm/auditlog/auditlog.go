// Copyright 2025 Canonical.

// The auditlog package provides business logic for handling audit log related methods.
package auditlog

import (
	"context"
	"strings"
	"time"

	"github.com/juju/names/v5"
	"github.com/juju/zaputil/zapctx"
	"go.uber.org/zap"

	"github.com/canonical/jimm/v3/internal/db"
	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/openfga"
	ofganames "github.com/canonical/jimm/v3/internal/openfga/names"
)

// auditLogManager provides a means to manage audit logs within JIMM.
type auditLogManager struct {
	store                 *db.Database
	authSvc               *openfga.OFGAClient
	jimmTag               names.ControllerTag
	retentionPeriodInDays int
}

// NewAuditLogManager returns a new auditLog manager that provides audit Log
// creation, and removal.
func NewAuditLogManager(store *db.Database, authSvc *openfga.OFGAClient, jimmTag names.ControllerTag, retentionDays int) (*auditLogManager, error) {
	if store == nil {
		return nil, errors.E("auditlog store cannot be nil")
	}
	if authSvc == nil {
		return nil, errors.E("auditlog authorisation service cannot be nil")
	}
	if jimmTag.String() == "" {
		return nil, errors.E("auditlog jimm tag cannot be empty")
	}
	return &auditLogManager{store, authSvc, jimmTag, retentionDays}, nil
}

// addAuditLogEntry causes an entry to be added the the audit log.
func (j *auditLogManager) AddAuditLogEntry(ale *dbmodel.AuditLogEntry) {
	ctx := context.Background()
	redactSensitiveParams(ale)
	if err := j.store.AddAuditLogEntry(ctx, ale); err != nil {
		zapctx.Error(ctx, "cannot store audit log entry", zap.Error(err), zap.Any("entry", *ale))
	}
}

var sensitiveMethods = map[string]struct{}{
	"login":                 {},
	"logindevice":           {},
	"getdevicesessiontoken": {},
	"loginwithsessiontoken": {},
	"addcredentials":        {},
	"updatecredentials":     {}}
var redactJSON = dbmodel.JSON(`{"params":"redacted"}`)

func redactSensitiveParams(ale *dbmodel.AuditLogEntry) {
	if ale.Params == nil {
		return
	}
	method := strings.ToLower(ale.FacadeMethod)
	if _, ok := sensitiveMethods[method]; ok {
		newRedactMessage := make(dbmodel.JSON, len(redactJSON))
		copy(newRedactMessage, redactJSON)
		ale.Params = newRedactMessage
	}
}

// FindAuditEvents returns audit events matching the given filter.
func (j *auditLogManager) FindAuditEvents(ctx context.Context, user *openfga.User, filter db.AuditLogFilter) ([]dbmodel.AuditLogEntry, error) {
	const op = errors.Op("jimm.FindAuditEvents")

	access := user.GetAuditLogViewerAccess(ctx, j.jimmTag)
	if access != ofganames.AuditLogViewerRelation {
		return nil, errors.E(op, errors.CodeUnauthorized, "unauthorized")
	}

	var entries []dbmodel.AuditLogEntry
	err := j.store.ForEachAuditLogEntry(ctx, filter, func(entry *dbmodel.AuditLogEntry) error {
		entries = append(entries, *entry)
		return nil
	})
	if err != nil {
		return nil, errors.E(op, err)
	}

	return entries, nil
}

// PurgeLogs removes all audit logs before the given timestamp. Only JIMM
// administrators can perform this operation. The number of logs purged is
// returned.
func (j *auditLogManager) PurgeLogs(ctx context.Context, user *openfga.User, before time.Time) (int64, error) {
	op := errors.Op("jimm.PurgeLogs")
	if !user.JimmAdmin {
		return 0, errors.E(op, errors.CodeUnauthorized, "unauthorized")
	}
	count, err := j.store.DeleteAuditLogsBefore(ctx, before)
	if err != nil {
		zapctx.Error(ctx, "failed to purge logs", zap.Error(err))
		return 0, errors.E(op, "failed to purge logs", err)
	}
	return count, nil
}
