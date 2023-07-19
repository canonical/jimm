// Copyright 2023 Canonical Ltd.

package jimm

import (
	"context"

	"github.com/CanonicalLtd/jimm/internal/errors"
	"github.com/CanonicalLtd/jimm/internal/openfga"
	"github.com/juju/zaputil/zapctx"
	"go.uber.org/zap"
)

func (j *JIMM) PurgeLogs(ctx context.Context, user *openfga.User, before string) (int64, error) {
	op := errors.Op("jimm.PurgeLogs")
	isJIMMAdmin, err := openfga.IsAdministrator(ctx, user, j.ResourceTag())
	if err != nil {
		zapctx.Error(ctx, "failed administrator check", zap.Error(err))
		return 0, errors.E(op, "failed administrator check", err)
	}
	if !isJIMMAdmin {
		return 0, errors.E(op, errors.CodeUnauthorized, "unauthorized")
	}
	count, err := j.Database.DeleteAuditLogsBefore(ctx, before)
	if err != nil {
		zapctx.Error(ctx, "failed to purge logs", zap.Error(err))
		return 0, errors.E(op, "failed to purge logs", err)
	}
	return count, nil
}
