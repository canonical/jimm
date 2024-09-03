// Copyright 2024 Canonical.

package rebac_admin

import (
	"context"

	"github.com/canonical/rebac-admin-ui-handlers/v1/resources"

	"github.com/canonical/jimm/v3/internal/jujuapi"
	"github.com/canonical/jimm/v3/internal/rebac_admin/utils"
)

// resourcesService implements the `resourcesService` interface.
type resourcesService struct {
	jimm jujuapi.JIMM
}

func newResourcesService(jimm jujuapi.JIMM) *resourcesService {
	return &resourcesService{
		jimm: jimm,
	}
}

// resourcesService defines an abstract backend to handle Resources related operations.
func (s *resourcesService) ListResources(ctx context.Context, params *resources.GetResourcesParams) (*resources.PaginatedResponse[resources.Resource], error) {
	_, err := utils.GetUserFromContext(ctx)
	if err != nil {
		return nil, err
	}
	return nil, nil
}
