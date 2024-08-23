package rebac_admin_test

import (
	"context"
	"fmt"

	rebac_handlers "github.com/canonical/rebac-admin-ui-handlers/v1"
	"github.com/canonical/rebac-admin-ui-handlers/v1/resources"
	gc "gopkg.in/check.v1"

	"github.com/canonical/jimm/v3/internal/jimmtest"
	"github.com/canonical/jimm/v3/internal/openfga"
	ofganames "github.com/canonical/jimm/v3/internal/openfga/names"
	"github.com/canonical/jimm/v3/internal/rebac_admin"
	"github.com/canonical/jimm/v3/pkg/api/params"
	jimmnames "github.com/canonical/jimm/v3/pkg/names"
	"github.com/juju/names/v5"
)

type identitiesSuite struct {
	jimmtest.JIMMSuite
}

var _ = gc.Suite(&identitiesSuite{})

func (s *identitiesSuite) TestIdentityPatchGroups(c *gc.C) {
	// initialization
	ctx := context.Background()
	ctx = rebac_handlers.ContextWithIdentity(ctx, s.AdminUser)
	identitySvc := rebac_admin.NewidentitiesService(s.JIMM)
	groupName := "group-test1"
	username := s.AdminUser.Name
	groupTag := s.AddGroup(c, groupName)

	// test add identity group
	changed, err := identitySvc.PatchIdentityGroups(ctx, username, []resources.IdentityGroupsPatchItem{{
		Group: groupTag.String(),
		Op:    resources.IdentityGroupsPatchItemOpAdd,
	}})
	c.Assert(err, gc.IsNil)
	c.Assert(changed, gc.Equals, true)

	// test user added to groups
	objUser, err := s.JIMM.FetchIdentity(ctx, username)
	c.Assert(err, gc.IsNil)
	tuples, _, err := s.JIMM.ListRelationshipTuples(ctx, s.AdminUser, params.RelationshipTuple{
		Object:       objUser.ResourceTag().String(),
		Relation:     ofganames.MemberRelation.String(),
		TargetObject: groupTag.String(),
	}, 10, "")
	c.Assert(err, gc.IsNil)
	c.Assert(len(tuples), gc.Equals, 1)
	c.Assert(groupTag.Id(), gc.Equals, tuples[0].Target.ID)

	// test user remove from group
	changed, err = identitySvc.PatchIdentityGroups(ctx, username, []resources.IdentityGroupsPatchItem{{
		Group: groupTag.String(),
		Op:    resources.IdentityGroupsPatchItemOpRemove,
	}})
	c.Assert(err, gc.IsNil)
	c.Assert(changed, gc.Equals, true)
	tuples, _, err = s.JIMM.ListRelationshipTuples(ctx, s.AdminUser, params.RelationshipTuple{
		Object:       objUser.ResourceTag().String(),
		Relation:     ofganames.MemberRelation.String(),
		TargetObject: groupTag.String(),
	}, 10, "")
	c.Assert(err, gc.IsNil)
	c.Assert(len(tuples), gc.Equals, 0)
}

func (s *identitiesSuite) TestIdentityGetGroups(c *gc.C) {
	// initialization
	ctx := context.Background()
	ctx = rebac_handlers.ContextWithIdentity(ctx, s.AdminUser)
	identitySvc := rebac_admin.NewidentitiesService(s.JIMM)
	username := s.AdminUser.Name
	groupsSize := 10
	groupsToAdd := make([]resources.IdentityGroupsPatchItem, groupsSize)
	groupTags := make([]jimmnames.GroupTag, groupsSize)
	for i := range groupsSize {
		groupName := fmt.Sprintf("group-test%d", i)
		groupTag := s.AddGroup(c, groupName)
		groupTags[i] = groupTag
		groupsToAdd[i] = resources.IdentityGroupsPatchItem{
			Group: groupTag.String(),
			Op:    resources.IdentityGroupsPatchItemOpAdd,
		}

	}
	changed, err := identitySvc.PatchIdentityGroups(ctx, username, groupsToAdd)
	c.Assert(err, gc.IsNil)
	c.Assert(changed, gc.Equals, true)

	// test list identity's groups with token pagination
	size := 3
	token := ""
	for i := 0; ; i += size {
		groups, err := identitySvc.GetIdentityGroups(ctx, username, &resources.GetIdentitiesItemGroupsParams{
			Size:      &size,
			NextToken: &token,
		})
		c.Assert(err, gc.IsNil)
		token = *groups.Next.PageToken
		for j := 0; j < len(groups.Data); j++ {
			c.Assert(groups.Data[j].Name, gc.Equals, groupTags[i+j].Id())
		}
		if *groups.Next.PageToken == "" {
			break
		}
	}
}

// TestIdentityEntitlements tests the listing of entitlements for a specific identityId.
// Setup: add controllers, models to a user and add the user to a group.
func (s *identitiesSuite) TestIdentityEntitlements(c *gc.C) {
	// add user to 3 controllers and 3 models
	ctx := context.Background()
	identitySvc := rebac_admin.NewidentitiesService(s.JIMM)
	groupTag := s.AddGroup(c, "test-group")
	user := names.NewUserTag("test-user@canonical.com")
	s.AddUser(c, user.Id())
	err := s.JIMM.OpenFGAClient.AddRelation(ctx, openfga.Tuple{
		Object:   ofganames.ConvertTag(user),
		Relation: ofganames.MemberRelation,
		Target:   ofganames.ConvertTag(groupTag),
	})
	c.Assert(err, gc.IsNil)
	tuple := openfga.Tuple{
		Object:   ofganames.ConvertTag(user),
		Relation: ofganames.AdministratorRelation,
	}
	var tuples []openfga.Tuple
	for i := range 3 {
		t := tuple
		t.Target = ofganames.ConvertTag(names.NewModelTag(fmt.Sprintf("test-model-%d", i)))
		tuples = append(tuples, t)
	}
	for i := range 3 {
		t := tuple
		t.Target = ofganames.ConvertTag(names.NewControllerTag(fmt.Sprintf("test-controller-%d", i)))
		tuples = append(tuples, t)
	}
	err = s.JIMM.OpenFGAClient.AddRelation(ctx, tuples...)
	c.Assert(err, gc.IsNil)

	ctx = rebac_handlers.ContextWithIdentity(ctx, s.AdminUser)
	emptyPageToken := ""
	req := resources.GetIdentitiesItemEntitlementsParams{NextPageToken: &emptyPageToken}
	var entitlements []resources.EntityEntitlement
	for {
		res, err := identitySvc.GetIdentityEntitlements(ctx, user.Id(), &req)
		c.Assert(err, gc.IsNil)
		c.Assert(res, gc.Not(gc.IsNil))
		entitlements = append(entitlements, res.Data...)
		if *res.Next.PageToken == "" {
			break
		}
		c.Assert(*res.Meta.PageToken, gc.Equals, *req.NextPageToken)
		c.Assert(*res.Next.PageToken, gc.Not(gc.Equals), "")
		req.NextPageToken = res.Next.PageToken
	}
	c.Assert(entitlements, gc.HasLen, 7)
	modelEntitlementCount := 0
	controllerEntitlementCount := 0
	groupEntitlementCount := 0
	for _, entitlement := range entitlements {
		switch entitlement.EntityType {
		case openfga.ModelType.String():
			c.Assert(entitlement.EntityId, gc.Matches, `test-model-\d`)
			c.Assert(entitlement.Entitlement, gc.Equals, ofganames.AdministratorRelation.String())
			modelEntitlementCount++
		case openfga.ControllerType.String():
			c.Assert(entitlement.EntityId, gc.Matches, `test-controller-\d`)
			c.Assert(entitlement.Entitlement, gc.Equals, ofganames.AdministratorRelation.String())
			controllerEntitlementCount++
		case openfga.GroupType.String():
			c.Assert(entitlement.Entitlement, gc.Equals, ofganames.MemberRelation.String())
			groupEntitlementCount++
		default:
			c.Logf("Unexpected entitlement found of type %s", entitlement.EntityType)
			c.FailNow()
		}
	}
	c.Assert(modelEntitlementCount, gc.Equals, 3)
	c.Assert(controllerEntitlementCount, gc.Equals, 3)
	c.Assert(groupEntitlementCount, gc.Equals, 1)
}
