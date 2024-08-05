package rebac_admin_test

import (
	"context"
	"fmt"

	gc "gopkg.in/check.v1"

	"github.com/canonical/jimm/v3/internal/jimmtest"
	"github.com/canonical/jimm/v3/internal/openfga"
	"github.com/canonical/jimm/v3/internal/openfga/names"
	"github.com/canonical/jimm/v3/internal/rebac_admin"
	"github.com/canonical/jimm/v3/pkg/api/params"
	jimmnames "github.com/canonical/jimm/v3/pkg/names"
	rebac_handlers "github.com/canonical/rebac-admin-ui-handlers/v1"
	"github.com/canonical/rebac-admin-ui-handlers/v1/resources"
)

type identitiesSuite struct {
	jimmtest.JIMMSuite
}

var _ = gc.Suite(&identitiesSuite{})

func (s *identitiesSuite) TestIdentityGroups(c *gc.C) {
	// initialization
	user := openfga.User{}
	user.JimmAdmin = true
	ctx := context.Background()
	ctx = rebac_handlers.ContextWithIdentity(ctx, &user)
	identitySvc := rebac_admin.NewidentitiesService(s.JIMM)
	username := "bob@canonical.com"
	s.AddAdminUser(c, username)
	groupsSize := 10
	groupsToAdd := make([]resources.IdentityGroupsPatchItem, groupsSize)
	groupTags := make([]jimmnames.GroupTag, groupsSize)
	for i := range 10 {
		groupName := fmt.Sprintf("group-test%d", i)
		groupTag := s.AddGroup(c, groupName)
		groupTags[i] = groupTag
		groupsToAdd[i] = resources.IdentityGroupsPatchItem{
			Group: groupTag.String(),
			Op:    resources.IdentityGroupsPatchItemOpAdd,
		}

	}

	// test add identity group
	changed, err := identitySvc.PatchIdentityGroups(ctx, username, groupsToAdd)
	c.Assert(err, gc.IsNil)
	c.Assert(changed, gc.Equals, true)

	// test user added to groups
	objUser, err := s.JIMM.FetchIdentity(ctx, username)
	c.Assert(err, gc.IsNil)
	tuples, _, err := s.JIMM.ListRelationshipTuples(ctx, &user, params.RelationshipTuple{
		Object:       objUser.ResourceTag().String(),
		Relation:     names.MemberRelation.String(),
		TargetObject: groupTags[0].String(),
	}, 10, "")
	c.Assert(err, gc.IsNil)
	c.Assert(len(tuples), gc.Equals, 1)
	c.Assert(groupTags[0].Id(), gc.Equals, tuples[0].Target.ID)

	// test list identity's groups with token pagination
	size := 3
	token := ""
	for i := 0; ; i += size {
		groups, err := identitySvc.GetIdentityGroups(ctx, username, &resources.GetIdentitiesItemGroupsParams{
			Size:      &size,
			NextToken: &token,
		})
		c.Assert(err, gc.IsNil)
		if *groups.Next.PageToken == "" {
			break
		}
		token = *groups.Next.PageToken
		for j := 0; j < len(groups.Data); j++ {
			c.Assert(groups.Data[j].Name, gc.Equals, groupTags[i+j].Id())
		}
	}
}
