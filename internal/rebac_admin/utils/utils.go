// Copyright 2024 Canonical Ltd.

package utils

import (
	"fmt"
	"time"

	"github.com/canonical/jimm/v3/internal/common/pagination"
	"github.com/canonical/jimm/v3/internal/openfga"
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

// CreatePaginationFilter return the pageSize, which will be equal to page if page is not nil
// and also returns a LimitOffset filter for use with paginated APIs.
func CreatePaginationFilter(size, page *int) (int, pagination.LimitOffsetPagination) {
	pageSize := -1
	offset := 0
	currentPage := 0
	if size != nil && page != nil {
		pageSize = *size
		currentPage = *page
		offset = pageSize * currentPage
	}
	return currentPage, pagination.NewOffsetFilter(pageSize, offset)
}
