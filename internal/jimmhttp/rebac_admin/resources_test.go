// Copyright 2024 Canonical.

package rebac_admin_test

import (
	"context"
	"testing"

	rebac_handlers "github.com/canonical/rebac-admin-ui-handlers/v1"
	"github.com/canonical/rebac-admin-ui-handlers/v1/resources"
	qt "github.com/frankban/quicktest"

	"github.com/canonical/jimm/v3/internal/common/pagination"
	"github.com/canonical/jimm/v3/internal/common/utils"
	"github.com/canonical/jimm/v3/internal/db"
	"github.com/canonical/jimm/v3/internal/jimmhttp/rebac_admin"
	"github.com/canonical/jimm/v3/internal/openfga"
	"github.com/canonical/jimm/v3/internal/testutils/jimmtest"
)

func TestListResources(t *testing.T) {
	c := qt.New(t)
	jimm := jimmtest.JIMM{
		ListResources_: func(ctx context.Context, user *openfga.User, filter pagination.LimitOffsetPagination, nameFilter, typeFilter string) ([]db.Resource, error) {
			return []db.Resource{}, nil
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
			size:       utils.IntToPointer(2),
			page:       utils.IntToPointer(0),
			nameFilter: utils.StringToPointer(""),
			typeFilter: utils.StringToPointer(""),
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
			typeFilter:       utils.StringToPointer("type-not-found"),
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
