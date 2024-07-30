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

func CreatePagination(params *resources.GetIdentitiesParams) (int, pagination.LimitOffsetPagination) {
	pageSize := -1
	offset := 0
	page := 0
	if params != nil {
		if params.Size != nil && params.Page != nil {
			pageSize = *params.Size
			page = *params.Page
			offset = pageSize * page
		}
	}
	return page, pagination.NewOffsetFilter(pageSize, offset)
}
