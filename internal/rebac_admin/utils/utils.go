// Copyright 2024 Canonical Ltd.

package utils

import (
	"fmt"
	"time"

	"github.com/canonical/jimm/internal/common/pagination"
	"github.com/canonical/jimm/internal/openfga"
	"github.com/canonical/rebac-admin-ui-handlers/v1/resources"
)

// ParseFromUserToIdentity parses openfga.User into resources.Identity .
func ParseFromUserToIdentity(user openfga.User) resources.Identity {
	id := fmt.Sprintf("%d", user.ID)
	joined := user.CreatedAt.Format(time.RFC3339)
	lastLogin := user.LastLogin.Time.Format(time.RFC3339)
	return resources.Identity{
		Email:     user.Name,
		Id:        &id,
		Joined:    &joined,
		LastLogin: &lastLogin,
		Source:    "",
	}
}

// CreatePagination returns the current page, the next page if exists, and the pagination.LimitOffsetPagination from the *resources.GetIdentitiesParams, a .
func CreatePagination(params *resources.GetIdentitiesParams, total int) (int, *int, pagination.LimitOffsetPagination) {
	pageSize := -1
	offset := 0
	page := 0
	var nextPage *int
	if params != nil {
		if params.Size != nil && params.Page != nil {
			pageSize = *params.Size
			page = *params.Page
			offset = pageSize * page
		}
	}
	if (page+1)*pageSize >= total {
		nextPage = nil
	} else {
		nPage := page + 1
		nextPage = &nPage
	}
	return page, nextPage, pagination.NewOffsetFilter(pageSize, offset)
}
