// Copyright 2025 Canonical.

package rebac_admin

import (
	"context"
	"fmt"

	rebac_handlers "github.com/canonical/rebac-admin-ui-handlers/v1"
	"github.com/canonical/rebac-admin-ui-handlers/v1/resources"
	"github.com/go-chi/chi/v5"
	"github.com/juju/zaputil/zapctx"
	"go.uber.org/zap"

	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/jimmhttp/rebac_admin/utils"
	"github.com/canonical/jimm/v3/internal/jujuapi"
	"github.com/canonical/jimm/v3/internal/logger"
)

func SetupBackend(ctx context.Context, jimm jujuapi.JIMM) (*rebac_handlers.ReBACAdminBackend, error) {

	securityEventLogger := &securityEventLogger{}

	rebacBackend, err := rebac_handlers.NewReBACAdminBackend(rebac_handlers.ReBACAdminBackendParams{
		Authenticator:           nil, // Authentication is handled by internal middleware.
		Entitlements:            newEntitlementService(),
		EntitlementsErrorMapper: securityEventLogger,
		Groups:                  newGroupService(jimm),
		GroupsErrorMapper:       securityEventLogger,
		Identities:              newidentitiesService(jimm),
		IdentitiesErrorMapper:   securityEventLogger,
		Resources:               newResourcesService(jimm),
		ResourcesErrorMapper:    securityEventLogger,
		Capabilities:            newCapabilitiesService(),
		CapabilitiesErrorMapper: securityEventLogger,
		Roles:                   newRoleService(jimm),
		RolesErrorMapper:        securityEventLogger,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create rebac admin backend: %w", err)
	}

	return rebacBackend, nil
}

type securityEventLogger struct{}

// MapError implements the ErrorResponseMapper interface.
// It logs security relevant events, such as unauthorized access attempts.
// It returns nil, as we do not want to override the default error mapping behaviour.
func (e *securityEventLogger) MapError(ctx context.Context, err error) *resources.Response {
	chiContext := chi.RouteContext(ctx)
	if errors.ErrorCode(err) == errors.CodeUnauthorized {
		user, err := utils.GetUserFromContext(ctx)
		if err != nil {
			zapctx.Error(ctx, "unable to fetch user from context", zap.Error(err))
			return nil
		}

		logger.LogUnauthorizedAccess(
			ctx,
			user.Name,
			fmt.Sprintf("attempted %s %s  in the ReBAC admin API", chiContext.RouteMethod, chiContext.RoutePath),
		)
	}
	return nil
}
