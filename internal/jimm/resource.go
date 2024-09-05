// Copyright 2024 Canonical.
package jimm

import (
	"context"

	"github.com/canonical/jimm/v3/internal/common/pagination"
	"github.com/canonical/jimm/v3/internal/db"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/openfga"
)

// ListResources returns a list of resources known to JIMM with a pagination filter.
func (j *JIMM) ListResources(ctx context.Context, user *openfga.User, filter pagination.LimitOffsetPagination) ([]db.Resource, error) {
	const op = errors.Op("jimm.ListResources")

	if !user.JimmAdmin {
		return nil, errors.E(op, errors.CodeUnauthorized, "unauthorized")
	}

	return j.Database.ListResources(ctx, filter.Limit(), filter.Offset())
}
