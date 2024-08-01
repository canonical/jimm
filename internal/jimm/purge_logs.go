// Copyright 2023 Canonical Ltd.

package jimm

import (
	"context"
	"time"

	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/openfga"
	"github.com/juju/zaputil/zapctx"
	"go.uber.org/zap"
)

// PurgeLogs removes all audit logs before the given timestamp. Only JIMM
// administrators can perform this operation. The number of logs purged is
// returned.
func (j *JIMM) PurgeLogs(ctx context.Context, user *openfga.User, before time.Time) (int64, error) {
	op := errors.Op("jimm.PurgeLogs")
	if !user.JimmAdmin {
		return 0, errors.E(op, errors.CodeUnauthorized, "unauthorized")
	}
	count, err := j.Database.DeleteAuditLogsBefore(ctx, before)
	if err != nil {
		zapctx.Error(ctx, "failed to purge logs", zap.Error(err))
		return 0, errors.E(op, "failed to purge logs", err)
	}
	return count, nil
}
