// Copyright 2024 canonical.

package utils

import (
	"github.com/canonical/jimm/v3/internal/common/pagination"
)

// CreateTokenPaginationFilter returns a token pagination filter based on the rebac admin request parameters.
//
// This is a helper method to parse the size and token from either query parameters or the request header.
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
