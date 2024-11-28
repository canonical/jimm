// Copyright 2024 Canonical.

package rebac_admin_test

import (
	"context"
	"testing"

	"github.com/canonical/rebac-admin-ui-handlers/v1/resources"
	qt "github.com/frankban/quicktest"

	"github.com/canonical/jimm/v3/internal/jimmhttp/rebac_admin"
)

func TestEntitlements(t *testing.T) {
	ctx := context.Background()
	c := qt.New(t)
	entitlementSvc := rebac_admin.NewEntitlementService()

	params := &resources.GetEntitlementsParams{}
	entitlements, err := entitlementSvc.ListEntitlements(ctx, params)
	c.Assert(err, qt.IsNil)
	c.Assert(entitlements, qt.HasLen, len(rebac_admin.EntitlementsList))

	match := "administrator"
	params.Filter = &match
	entitlements, err = entitlementSvc.ListEntitlements(ctx, params)
	c.Assert(err, qt.IsNil)
	c.Assert(entitlements, qt.HasLen, 20)

	match = "cloud"
	params.Filter = &match
	entitlements, err = entitlementSvc.ListEntitlements(ctx, params)
	c.Assert(err, qt.IsNil)
	c.Assert(entitlements, qt.HasLen, 8)

	match = "#member"
	params.Filter = &match
	entitlements, err = entitlementSvc.ListEntitlements(ctx, params)
	c.Assert(err, qt.IsNil)
	c.Assert(entitlements, qt.HasLen, 0)
}
