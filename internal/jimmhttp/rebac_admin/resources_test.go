// Copyright 2025 Canonical.

package rebac_admin_test

import (
	"context"
	"testing"

	rebac_handlers "github.com/canonical/rebac-admin-ui-handlers/v1"
	"github.com/canonical/rebac-admin-ui-handlers/v1/resources"
	qt "github.com/frankban/quicktest"

	"github.com/canonical/jimm/v3/internal/common/pagination"
	"github.com/canonical/jimm/v3/internal/db"
	"github.com/canonical/jimm/v3/internal/jimmhttp/rebac_admin"
	"github.com/canonical/jimm/v3/internal/jujuapi"
	"github.com/canonical/jimm/v3/internal/openfga"
	"github.com/canonical/jimm/v3/internal/testutils/jimmtest"
	"github.com/canonical/jimm/v3/internal/testutils/jimmtest/mocks"
)

func TestListResources(t *testing.T) {
	c := qt.New(t)
	permissionManager := mocks.PermissionManager{
		ListResources_: func(ctx context.Context, user *openfga.User, filter pagination.LimitOffsetPagination, nameFilter, typeFilter string) ([]db.Resource, error) {
			return []db.Resource{}, nil
		},
	}
	jimm := jimmtest.JIMM{
		PermissionManager_: func() jujuapi.PermissionManager {
			return &permissionManager
		},
	}
	user := openfga.User{}
	user.JimmAdmin = true
	ctx := context.Background()
	ctx = rebac_handlers.ContextWithIdentity(ctx, &user)
	resourcesSvc := rebac_admin.NewResourcesService(&jimm)

	testCases := []struct {
		desc             string
		size             *int
		page             *int
		nameFilter       *string
		typeFilter       *string
		expectErrorMatch string
	}{
		{
			desc:       "test good",
			size:       new(2),
			page:       new(0),
			nameFilter: new(""),
			typeFilter: new(""),
		},
		{
			desc:       "test good with all params set to nil",
			size:       nil,
			page:       nil,
			nameFilter: nil,
			typeFilter: nil,
		},
		{
			desc:             "test with not valid type filter",
			size:             nil,
			page:             nil,
			nameFilter:       nil,
			typeFilter:       new("type-not-found"),
			expectErrorMatch: ".*this resource type is not supported.*",
		},
	}
	for _, t := range testCases {
		c.Run(t.desc, func(c *qt.C) {
			_, err := resourcesSvc.ListResources(ctx, &resources.GetResourcesParams{
				Page:       t.page,
				Size:       t.size,
				EntityType: t.typeFilter,
				EntityName: t.nameFilter,
			})
			if t.expectErrorMatch != "" {
				c.Assert(err, qt.ErrorMatches, t.expectErrorMatch)
			} else {
				c.Assert(err, qt.IsNil)
			}
		})
	}

}
