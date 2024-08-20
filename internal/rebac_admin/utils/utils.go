// Copyright 2024 Canonical Ltd.

package utils

import (
	"fmt"
	"time"

	"github.com/canonical/rebac-admin-ui-handlers/v1/resources"

	"github.com/canonical/jimm/v3/internal/common/pagination"
	"github.com/canonical/jimm/v3/internal/openfga"
)

// FromUserToIdentity parses openfga.User into resources.Identity .
func FromUserToIdentity(user openfga.User) resources.Identity {
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

// CreateTokenPaginationFilter returns a token pagination filter based on the rebac admin request parameters.
func CreateTokenPaginationFilter(size *int, token, tokenFromHeader *string) pagination.OpenFGAPagination {
	pageSize := 0
	if size != nil {
		pageSize = *size
	}
	var pageToken string
	if tokenFromHeader != nil {
		pageToken = *tokenFromHeader
	} else if token != nil {
		pageToken = *token
	}
	return pagination.NewOpenFGAFilter(pageSize, pageToken)
}
