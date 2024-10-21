// Copyright 2024 Canonical.

package rebac_admin

import (
	"context"

	v1 "github.com/canonical/rebac-admin-ui-handlers/v1"
	"github.com/canonical/rebac-admin-ui-handlers/v1/resources"

	"github.com/canonical/jimm/v3/internal/common/pagination"
	"github.com/canonical/jimm/v3/internal/db"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/jimmhttp/rebac_admin/utils"
	"github.com/canonical/jimm/v3/internal/jujuapi"
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
	namePrefixFilter, typeFilter := utils.GetNameAndTypeResourceFilter(params.EntityName, params.EntityType)
	if typeFilter != "" {
		typeFilter, err = validateAndConvertResourceFilter(typeFilter)
		if err != nil {
			return nil, v1.NewInvalidRequestError(err.Error())
		}
	}

	res, err := s.jimm.ListResources(ctx, user, pagination, namePrefixFilter, typeFilter)
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

// getNextPageAndResources checks for the expectedPageSize of the resources.
// If there is enough records we return the records minus 1 and advice the consumer there is another page.
// Otherwise we return the records we have and set next page as empty.
func getNextPageAndResources(currentPage, expectedPageSize int, resources []db.Resource) (*int, []db.Resource) {
	var nextPage *int
	if len(resources) > 0 && len(resources) == expectedPageSize {
		nPage := currentPage + 1
		nextPage = &nPage
		resources = resources[:len(resources)-1]
	}
	return nextPage, resources
}

// validateAndConvertResourceFilter checks the typeFilter in the request and converts it to a valid key that is used to retrieved
// the right resources from the db.
func validateAndConvertResourceFilter(typeFilter string) (string, error) {
	switch typeFilter {
	case ApplicationOffer:
		return db.ApplicationOffersQueryKey, nil
	case Cloud:
		return db.CloudsQueryKey, nil
	case Controller:
		return db.ControllersQueryKey, nil
	case Model:
		return db.ModelsQueryKey, nil
	case ServiceAccount:
		return db.ServiceAccountQueryKey, nil
	default:
		return "", errors.E("this resource type is not supported")

	}
}
