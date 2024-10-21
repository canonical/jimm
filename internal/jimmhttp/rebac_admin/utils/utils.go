// Copyright 2024 Canonical.

package utils

import (
	"time"

	"github.com/canonical/rebac-admin-ui-handlers/v1/resources"
	"github.com/juju/names/v5"

	"github.com/canonical/jimm/v3/internal/common/pagination"
	"github.com/canonical/jimm/v3/internal/db"
	"github.com/canonical/jimm/v3/internal/openfga"
	jimmnames "github.com/canonical/jimm/v3/pkg/names"
)

// FromUserToIdentity parses openfga.User into resources.Identity.
func FromUserToIdentity(user openfga.User) resources.Identity {
	joined := user.CreatedAt.Format(time.RFC3339)
	lastLogin := user.LastLogin.Time.Format(time.RFC3339)
	return resources.Identity{
		Email:     user.Name,
		Id:        &user.Name,
		Joined:    &joined,
		LastLogin: &lastLogin,
		Source:    "",
	}
}

// ToRebacResource parses db.Resource into resources.Resource.
func ToRebacResource(res db.Resource) resources.Resource {
	r := resources.Resource{
		Entity: resources.Entity{
			Id:   res.ID.String,
			Name: res.Name,
			Type: res.Type,
		},
	}
	// the parent is populated only for models and application offers.
	// the parent type is set empty from the query.
	if res.ParentType != "" {
		r.Parent = &resources.Entity{
			Id:   res.ParentId.String,
			Name: res.ParentName,
			Type: res.ParentType,
		}
	}
	return r
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

// ValidateDecomposedTag validates that a kind and ID are a valid Juju or JIMM tag.
func ValidateDecomposedTag(kind string, id string) (names.Tag, error) {
	rawTag := kind + "-" + id
	return jimmnames.ParseTag(rawTag)
}

func GetNameAndTypeResourceFilter(nFilter, eFilter *string) (nameFilter, typeFilter string) {
	if nFilter != nil {
		nameFilter = *nFilter
	}
	if eFilter != nil {
		typeFilter = *eFilter
	}
	return nameFilter, typeFilter
}
