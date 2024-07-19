package rebac_admin

import (
	"context"

	"github.com/canonical/jimm/internal/errors"
	rebachandlers "github.com/canonical/rebac-admin-ui-handlers/v1"
	"github.com/juju/zaputil/zapctx"
	"go.uber.org/zap"
)

func SetupBackend(ctx context.Context) (*rebachandlers.ReBACAdminBackend, error) {
	const op = errors.Op("setupRebacBackend")

	rebacBackend, err := rebachandlers.NewReBACAdminBackend(rebachandlers.ReBACAdminBackendParams{
		Authenticator: &Authenticator{},
	})
	if err != nil {
		zapctx.Error(ctx, "failed to create rebac admin backend", zap.Error(err))
		return nil, errors.E(op, err, "failed to create rebac admin backend")
	}

	return rebacBackend, nil
}
