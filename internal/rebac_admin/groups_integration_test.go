// Copyright 2024 Canonical Ltd.

package rebac_admin_test

import (
	"context"
	"fmt"

	gc "gopkg.in/check.v1"

	"github.com/canonical/jimm/v3/internal/jimmtest"
	"github.com/canonical/jimm/v3/internal/openfga"
	ofganames "github.com/canonical/jimm/v3/internal/openfga/names"
	"github.com/canonical/jimm/v3/internal/rebac_admin"
	jimmnames "github.com/canonical/jimm/v3/pkg/names"
	rebac_handlers "github.com/canonical/rebac-admin-ui-handlers/v1"
	"github.com/canonical/rebac-admin-ui-handlers/v1/resources"
	"github.com/juju/names/v5"
)

type rebacAdminSuite struct {
	jimmtest.JIMMSuite
	groupSvc *rebac_admin.GroupsService
}

func (s *rebacAdminSuite) SetUpTest(c *gc.C) {
	s.JIMMSuite.SetUpTest(c)
	s.groupSvc = rebac_admin.NewGroupService(s.JIMM)
}

var _ = gc.Suite(&rebacAdminSuite{})

func (s rebacAdminSuite) TestGetGroupIdentitiesIntegration(c *gc.C) {
	ctx := context.Background()
	group, err := s.JIMM.AddGroup(ctx, s.AdminUser, "test-group")
	c.Assert(err, gc.IsNil)
	tuple := openfga.Tuple{
		Relation: ofganames.MemberRelation,
		Target:   ofganames.ConvertTag(jimmnames.NewGroupTag(group.UUID)),
	}
	var tuples []openfga.Tuple
	for i := range 10 {
		t := tuple
		t.Object = ofganames.ConvertTag(names.NewUserTag(fmt.Sprintf("foo%d@canonical.com", i)))
		tuples = append(tuples, t)
	}
	err = s.JIMM.OpenFGAClient.AddRelation(ctx, tuples...)
	c.Assert(err, gc.IsNil)
	// Request Subset of items
	pageSize := 5
	params := &resources.GetGroupsItemIdentitiesParams{Size: &pageSize}
	ctx = rebac_handlers.ContextWithIdentity(ctx, s.AdminUser)
	res, err := s.groupSvc.GetGroupIdentities(ctx, group.UUID, params)
	c.Assert(err, gc.IsNil)
	c.Assert(res, gc.Not(gc.IsNil))
	c.Assert(res.Meta.Size, gc.Equals, 5)
	c.Assert(*res.Meta.PageToken, gc.Equals, "")
	c.Assert(*res.Next.PageToken, gc.Not(gc.Equals), "")
	c.Assert(res.Data, gc.HasLen, 5)
	c.Assert(res.Data[0].Email, gc.Equals, "foo0@canonical.com")

	// Request all items, no next page.
	allItems := &resources.GetGroupsItemIdentitiesParams{}
	res, err = s.groupSvc.GetGroupIdentities(ctx, group.UUID, allItems)
	c.Assert(err, gc.IsNil)
	c.Assert(res, gc.Not(gc.IsNil))
	c.Assert(*res.Next.PageToken, gc.Equals, "")
}

func (s rebacAdminSuite) TestPatchGroupIdentitiesIntegration(c *gc.C) {
	ctx := context.Background()
	group, err := s.JIMM.AddGroup(ctx, s.AdminUser, "test-group")
	c.Assert(err, gc.IsNil)
	tuple := openfga.Tuple{
		Relation: ofganames.MemberRelation,
		Target:   ofganames.ConvertTag(jimmnames.NewGroupTag(group.UUID)),
	}
	var tuples []openfga.Tuple
	for i := range 2 {
		t := tuple
		t.Object = ofganames.ConvertTag(names.NewUserTag(fmt.Sprintf("foo%d@canonical.com", i)))
		tuples = append(tuples, t)
	}
	err = s.JIMM.OpenFGAClient.AddRelation(ctx, tuples...)
	c.Assert(err, gc.IsNil)
	allowed, err := s.JIMM.OpenFGAClient.CheckRelation(ctx, tuples[0], false)
	c.Assert(err, gc.IsNil)
	c.Assert(allowed, gc.Equals, true)
	// Above we have added 2 users to the group, below, we will request those 2 users to be removed
	// and add 2 different users to the group, in the same request.
	entitlementPatches := []resources.GroupIdentitiesPatchItem{
		{Identity: "foo0@canonical.com", Op: resources.GroupIdentitiesPatchItemOpRemove},
		{Identity: "foo1@canonical.com", Op: resources.GroupIdentitiesPatchItemOpRemove},
		{Identity: "foo2@canonical.com", Op: resources.GroupIdentitiesPatchItemOpAdd},
		{Identity: "foo3@canonical.com", Op: resources.GroupIdentitiesPatchItemOpAdd},
	}
	ctx = rebac_handlers.ContextWithIdentity(ctx, s.AdminUser)
	res, err := s.groupSvc.PatchGroupIdentities(ctx, group.UUID, entitlementPatches)
	c.Assert(err, gc.IsNil)
	c.Assert(res, gc.Equals, true)

	allowed, err = s.JIMM.OpenFGAClient.CheckRelation(ctx, tuples[0], false)
	c.Assert(err, gc.IsNil)
	c.Assert(allowed, gc.Equals, false)
	newTuple := tuples[0]
	newTuple.Object = ofganames.ConvertTag(names.NewUserTag("foo2@canonical.com"))
	allowed, err = s.JIMM.OpenFGAClient.CheckRelation(ctx, newTuple, false)
	c.Assert(err, gc.IsNil)
	c.Assert(allowed, gc.Equals, true)
}
