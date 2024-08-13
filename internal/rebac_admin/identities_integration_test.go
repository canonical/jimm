package rebac_admin_test

import (
	"context"
	"fmt"

	rebac_handlers "github.com/canonical/rebac-admin-ui-handlers/v1"
	"github.com/canonical/rebac-admin-ui-handlers/v1/resources"
	gc "gopkg.in/check.v1"

	"github.com/canonical/jimm/v3/internal/jimmtest"
	"github.com/canonical/jimm/v3/internal/openfga/names"
	"github.com/canonical/jimm/v3/internal/rebac_admin"
	"github.com/canonical/jimm/v3/pkg/api/params"
	jimmnames "github.com/canonical/jimm/v3/pkg/names"
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
		Relation:     names.MemberRelation.String(),
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
		Relation:     names.MemberRelation.String(),
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
