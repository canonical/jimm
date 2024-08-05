// Copyright 2024 Canonical Ltd.

package rebac_admin

import (
	"context"

	"github.com/juju/zaputil/zapctx"
	"go.uber.org/zap"

	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/jujuapi"
	rebac_handlers "github.com/canonical/rebac-admin-ui-handlers/v1"
)

func SetupBackend(ctx context.Context, jimm jujuapi.JIMM) (*rebac_handlers.ReBACAdminBackend, error) {
	const op = errors.Op("rebac_admin.SetupBackend")

	rebacBackend, err := rebac_handlers.NewReBACAdminBackend(rebac_handlers.ReBACAdminBackendParams{
		Authenticator: nil, // Authentication is handled by internal middleware.
		Entitlements:  newEntitlementService(),
		Groups:        newGroupService(jimm),
		Identities:    newidentitiesService(jimm),
	})
	if err != nil {
		zapctx.Error(ctx, "failed to create rebac admin backend", zap.Error(err))
		return nil, errors.E(op, err, "failed to create rebac admin backend")
	}

	return rebacBackend, nil
}
