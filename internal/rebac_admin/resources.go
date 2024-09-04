// Copyright 2024 Canonical.

package rebac_admin

import (
	"context"

	"github.com/canonical/rebac-admin-ui-handlers/v1/resources"

	"github.com/canonical/jimm/v3/internal/common/pagination"
	"github.com/canonical/jimm/v3/internal/jujuapi"
	"github.com/canonical/jimm/v3/internal/rebac_admin/utils"
)

type resourcesService struct {
	jimm jujuapi.JIMM
}

func newResourcesService(jimm jujuapi.JIMM) *resourcesService {
	return &resourcesService{
		jimm: jimm,
	}
}

// ListResources returns a page of Resource objects of at least `size` elements if available.
func (s *resourcesService) ListResources(ctx context.Context, params *resources.GetResourcesParams) (*resources.PaginatedResponse[resources.Resource], error) {
	user, err := utils.GetUserFromContext(ctx)
	if err != nil {
		return nil, err
	}
	page, pageSize, pagination := pagination.CreatePaginationWithoutTotal(params.Size, params.Page)
	res, err := s.jimm.ListResources(ctx, user, pagination)
	if err != nil {
		return nil, err
	}
	// We fetch one record more than the page size. Then, we set the next page if we have this many records.
	// Otherwise next page is empty.
	var nextPage *int
	if len(res) == pageSize {
		nPage := page + 1
		nextPage = &nPage
		res = res[:len(res)-1]
	}
	rRes := make([]resources.Resource, len(res))
	for i, u := range res {
		rRes[i] = utils.FromDbResourcesToResources(u)
	}

	return &resources.PaginatedResponse[resources.Resource]{
		Data: rRes,
		Meta: resources.ResponseMeta{
			Page:  &page,
			Size:  len(rRes),
			Total: nil,
		},
		Next: resources.Next{
			Page: nextPage,
		},
	}, nil
}
