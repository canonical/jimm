// Copyright 2024 Canonical Ltd.

package rebac_admin

import (
	"context"

	"github.com/canonical/jimm/internal/errors"
	"github.com/canonical/jimm/internal/jimm"
	rebac_handlers "github.com/canonical/rebac-admin-ui-handlers/v1"
	"github.com/juju/zaputil/zapctx"
	"go.uber.org/zap"
)

func SetupBackend(ctx context.Context, jimm *jimm.JIMM) (*rebac_handlers.ReBACAdminBackend, error) {
	const op = errors.Op("rebac_admin.SetupBackend")

	rebacBackend, err := rebac_handlers.NewReBACAdminBackend(rebac_handlers.ReBACAdminBackendParams{
		Authenticator: &Authenticator{},
		Entitlements:  newEntitlementService(),
		Groups:        newGroupService(jimm),
	})
	if err != nil {
		zapctx.Error(ctx, "failed to create rebac admin backend", zap.Error(err))
		return nil, errors.E(op, err, "failed to create rebac admin backend")
	}

	return rebacBackend, nil
}
