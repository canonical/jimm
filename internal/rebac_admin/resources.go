// Copyright 2024 Canonical.

package rebac_admin

import (
	"context"

	"github.com/canonical/rebac-admin-ui-handlers/v1/resources"

	"github.com/canonical/jimm/v3/internal/common/pagination"
	"github.com/canonical/jimm/v3/internal/db"
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
	currentPage, expectedPageSize, pagination := pagination.CreatePaginationWithoutTotal(params.Size, params.Page)
	res, err := s.jimm.ListResources(ctx, user, pagination)
	if err != nil {
		return nil, err
	}
	nextPage, res := getNextPageAndResources(currentPage, expectedPageSize, res)
	rRes := make([]resources.Resource, len(res))
	for i, u := range res {
		rRes[i] = utils.ToRebacResource(u)
	}

	return &resources.PaginatedResponse[resources.Resource]{
		Data: rRes,
		Meta: resources.ResponseMeta{
			Page:  &currentPage,
			Size:  len(rRes),
			Total: nil,
		},
		Next: resources.Next{
			Page: nextPage,
		},
	}, nil
}

// We fetch one record more than the page size.
// Then, we set the next page if we have this many records.
// If it does we return the records minus 1 and advide the consumer there is another page.
// Otherwise we return the records we have and set next page as empty.
func getNextPageAndResources(currentPage, expectedPageSize int, resources []db.Resource) (*int, []db.Resource) {
	var nextPage *int
	if len(resources) == expectedPageSize {
		nPage := currentPage + 1
		nextPage = &nPage
		resources = resources[:len(resources)-1]
	}
	return nextPage, resources
}
