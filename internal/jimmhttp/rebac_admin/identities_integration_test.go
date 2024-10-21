// Copyright 2024 Canonical.
package rebac_admin_test

import (
	"context"
	"fmt"

	rebac_handlers "github.com/canonical/rebac-admin-ui-handlers/v1"
	"github.com/canonical/rebac-admin-ui-handlers/v1/resources"
	"github.com/juju/names/v5"
	gc "gopkg.in/check.v1"

	"github.com/canonical/jimm/v3/internal/jimmhttp/rebac_admin"
	"github.com/canonical/jimm/v3/internal/openfga"
	ofganames "github.com/canonical/jimm/v3/internal/openfga/names"
	"github.com/canonical/jimm/v3/internal/testutils/jimmtest"
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
	// initialization
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

	// test
	ctx = rebac_handlers.ContextWithIdentity(ctx, s.AdminUser)
	emptyPageToken := ""
	req := resources.GetIdentitiesItemEntitlementsParams{NextPageToken: &emptyPageToken}
	var entitlements []resources.EntityEntitlement
	for {
		res, err := identitySvc.GetIdentityEntitlements(ctx, user.Id(), &req)
		c.Assert(err, gc.IsNil)
		c.Assert(res, gc.Not(gc.IsNil))
		entitlements = append(entitlements, res.Data...)
		if res.Next.PageToken == nil || *res.Next.PageToken == "" {
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

// patchIdentitiesEntitlementTestEnv is used to create entries in JIMM's database.
// The rebacAdminSuite does not spin up a Juju controller so we cannot use
// regular JIMM methods to create resources. It is also necessary to have resources
// present in the database in order for ListRelationshipTuples to work correctly.
const patchIdentitiesEntitlementTestEnv = `clouds:
- name: test-cloud
  type: test-provider
  regions:
  - name: test-cloud-region
cloud-credentials:
- owner: alice@canonical.com
  name: cred-1
  cloud: test-cloud
controllers:
- name: controller-1
  uuid: 00000001-0000-0000-0000-000000000001
  cloud: test-cloud
  region: test-cloud-region
models:
- name: model-1
  uuid: 00000002-0000-0000-0000-000000000001
  controller: controller-1
  cloud: test-cloud
  region: test-cloud-region
  cloud-credential: cred-1
  owner: alice@canonical.com
- name: model-2
  uuid: 00000002-0000-0000-0000-000000000002
  controller: controller-1
  cloud: test-cloud
  region: test-cloud-region
  cloud-credential: cred-1
  owner: alice@canonical.com
- name: model-3
  uuid: 00000003-0000-0000-0000-000000000003
  controller: controller-1
  cloud: test-cloud
  region: test-cloud-region
  cloud-credential: cred-1
  owner: alice@canonical.com
- name: model-4
  uuid: 00000004-0000-0000-0000-000000000004
  controller: controller-1
  cloud: test-cloud
  region: test-cloud-region
  cloud-credential: cred-1
  owner: alice@canonical.com
`

// TestPatchIdentityEntitlements tests the patching of entitlements for a specific identityId,
// adding and removing relations after the setup.
// Setup: add user to a group, and add models to the user.
func (s *identitiesSuite) TestPatchIdentityEntitlements(c *gc.C) {
	// initialization
	ctx := context.Background()
	identitySvc := rebac_admin.NewidentitiesService(s.JIMM)
	tester := jimmtest.GocheckTester{C: c}
	env := jimmtest.ParseEnvironment(tester, patchIdentitiesEntitlementTestEnv)
	env.PopulateDB(tester, s.JIMM.Database)
	oldModels := []string{env.Models[0].UUID, env.Models[1].UUID}
	newModels := []string{env.Models[2].UUID, env.Models[3].UUID}
	user := names.NewUserTag("test-user@canonical.com")
	s.AddUser(c, user.Id())
	tuple := openfga.Tuple{
		Object:   ofganames.ConvertTag(user),
		Relation: ofganames.AdministratorRelation,
	}

	var tuples []openfga.Tuple
	for i := range 2 {
		t := tuple
		t.Target = ofganames.ConvertTag(names.NewModelTag(oldModels[i]))
		tuples = append(tuples, t)
	}
	err := s.JIMM.OpenFGAClient.AddRelation(ctx, tuples...)
	c.Assert(err, gc.IsNil)
	allowed, err := s.JIMM.OpenFGAClient.CheckRelation(ctx, tuples[0], false)
	c.Assert(err, gc.IsNil)
	c.Assert(allowed, gc.Equals, true)
	// Above we have added granted the user with administrator permission to 2 models.
	// Below, we will request those 2 relations to be removed and add 2 different relations.

	entitlementPatches := []resources.IdentityEntitlementsPatchItem{
		{Entitlement: resources.EntityEntitlement{
			Entitlement: ofganames.AdministratorRelation.String(),
			EntityId:    newModels[0],
			EntityType:  openfga.ModelType.String(),
		}, Op: resources.IdentityEntitlementsPatchItemOpAdd},
		{Entitlement: resources.EntityEntitlement{
			Entitlement: ofganames.AdministratorRelation.String(),
			EntityId:    newModels[1],
			EntityType:  openfga.ModelType.String(),
		}, Op: resources.IdentityEntitlementsPatchItemOpAdd},
		{Entitlement: resources.EntityEntitlement{
			Entitlement: ofganames.AdministratorRelation.String(),
			EntityId:    oldModels[0],
			EntityType:  openfga.ModelType.String(),
		}, Op: resources.IdentityEntitlementsPatchItemOpRemove},
		{Entitlement: resources.EntityEntitlement{
			Entitlement: ofganames.AdministratorRelation.String(),
			EntityId:    oldModels[1],
			EntityType:  openfga.ModelType.String(),
		}, Op: resources.IdentityEntitlementsPatchItemOpRemove},
	}
	ctx = rebac_handlers.ContextWithIdentity(ctx, s.AdminUser)
	res, err := identitySvc.PatchIdentityEntitlements(ctx, user.Id(), entitlementPatches)
	c.Assert(err, gc.IsNil)
	c.Assert(res, gc.Equals, true)

	for i := range 2 {
		exists, err := s.JIMM.OpenFGAClient.CheckRelation(ctx, tuples[i], false)
		c.Assert(err, gc.IsNil)
		c.Assert(exists, gc.Equals, false)
	}
	for i := range 2 {
		newTuple := tuples[0]
		newTuple.Target = ofganames.ConvertTag(names.NewModelTag(newModels[i]))
		allowed, err = s.JIMM.OpenFGAClient.CheckRelation(ctx, newTuple, false)
		c.Assert(err, gc.IsNil)
		c.Assert(allowed, gc.Equals, true)
	}
}
