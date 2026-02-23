// Copyright 2025 Canonical.

package rebac_admin_test

import (
	"fmt"
	"testing"

	rebac_handlers "github.com/canonical/rebac-admin-ui-handlers/v1"
	"github.com/canonical/rebac-admin-ui-handlers/v1/resources"
	qt "github.com/frankban/quicktest"
	"github.com/juju/names/v5"

	"github.com/canonical/jimm/v3/internal/jimmhttp/rebac_admin"
	"github.com/canonical/jimm/v3/internal/jujuapi"
	"github.com/canonical/jimm/v3/internal/openfga"
	ofganames "github.com/canonical/jimm/v3/internal/openfga/names"
	"github.com/canonical/jimm/v3/internal/testutils/jimmtest"
	jimmnames "github.com/canonical/jimm/v3/pkg/names"
)

type groupsTest struct {
	jimmtest.JIMMEnv
	groupSvc *rebac_admin.GroupsService
}

func SetupGroupsTest(c *qt.C) (s *groupsTest) {
	jimmEnv := jimmtest.SetupJimmEnv(c)
	groupSvc := rebac_admin.NewGroupService(jujuapi.NewJIMMAdapter(jimmEnv.JIMM))
	s = &groupsTest{
		JIMMEnv:  jimmEnv,
		groupSvc: groupSvc,
	}
	return s
}

func TestListGroupsWithFilterIntegration(t *testing.T) {
	c := qt.New(t)
	s := SetupGroupsTest(c)

	ctx := c.Context()
	for i := range 10 {
		_, err := s.JIMM.GroupManager.AddGroup(ctx, s.AdminUser, fmt.Sprintf("test-group-filter-%d", i))
		c.Assert(err, qt.IsNil)
	}

	ctx = rebac_handlers.ContextWithIdentity(ctx, s.AdminUser)
	pageSize := 5
	page := 0
	params := &resources.GetGroupsParams{Size: &pageSize, Page: &page}
	res, err := s.groupSvc.ListGroups(ctx, params)
	c.Assert(err, qt.IsNil)
	c.Assert(res, qt.Not(qt.IsNil))
	c.Assert(res.Meta.Size, qt.Equals, 5)

	match := "group-filter-1"
	params.Filter = &match
	res, err = s.groupSvc.ListGroups(ctx, params)
	c.Assert(err, qt.IsNil)
	c.Assert(res, qt.Not(qt.IsNil))
	c.Assert(len(res.Data), qt.Equals, 1)

	match = "group"
	params.Filter = &match
	res, err = s.groupSvc.ListGroups(ctx, params)
	c.Assert(err, qt.IsNil)
	c.Assert(res, qt.Not(qt.IsNil))
	c.Assert(len(res.Data), qt.Equals, pageSize)
}

func TestGetGroupIdentitiesIntegration(t *testing.T) {
	c := qt.New(t)
	s := SetupGroupsTest(c)

	ctx := c.Context()
	group, err := s.JIMM.GroupManager.AddGroup(ctx, s.AdminUser, "test-group")
	c.Assert(err, qt.IsNil)
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
	c.Assert(err, qt.IsNil)
	// Request Subset of items
	pageSize := 5
	params := &resources.GetGroupsItemIdentitiesParams{Size: &pageSize}
	ctx = rebac_handlers.ContextWithIdentity(ctx, s.AdminUser)
	res, err := s.groupSvc.GetGroupIdentities(ctx, group.UUID, params)
	c.Assert(err, qt.IsNil)
	c.Assert(res, qt.Not(qt.IsNil))
	c.Assert(res.Meta.Size, qt.Equals, 5)
	c.Assert(*res.Meta.PageToken, qt.Equals, "")
	c.Assert(*res.Next.PageToken, qt.Not(qt.Equals), "")
	c.Assert(res.Data, qt.HasLen, 5)
	c.Assert(res.Data[0].Email, qt.Matches, `foo\d@canonical\.com`)

	// Request next page
	params.NextPageToken = res.Next.PageToken
	res, err = s.groupSvc.GetGroupIdentities(ctx, group.UUID, params)
	c.Assert(err, qt.IsNil)
	c.Assert(res, qt.Not(qt.IsNil))
	c.Assert(res.Meta.Size, qt.Equals, 5)
	c.Assert(*res.Meta.PageToken, qt.Equals, *params.NextPageToken)
	c.Assert(res.Next.PageToken, qt.IsNil)
	c.Assert(res.Data, qt.HasLen, 5)
	c.Assert(res.Data[0].Email, qt.Matches, `foo\d@canonical\.com`)

	// Request all items, no next page.
	allItems := &resources.GetGroupsItemIdentitiesParams{}
	res, err = s.groupSvc.GetGroupIdentities(ctx, group.UUID, allItems)
	c.Assert(err, qt.IsNil)
	c.Assert(res, qt.Not(qt.IsNil))
	c.Assert(res.Next.PageToken, qt.IsNil)
}

func TestPatchGroupIdentitiesIntegration(t *testing.T) {
	c := qt.New(t)
	s := SetupGroupsTest(c)

	ctx := c.Context()
	group, err := s.JIMM.GroupManager.AddGroup(ctx, s.AdminUser, "test-group")
	c.Assert(err, qt.IsNil)
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
	c.Assert(err, qt.IsNil)
	allowed, err := s.JIMM.OpenFGAClient.CheckRelation(ctx, tuples[0], false)
	c.Assert(err, qt.IsNil)
	c.Assert(allowed, qt.Equals, true)
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
	c.Assert(err, qt.IsNil)
	c.Assert(res, qt.Equals, true)

	allowed, err = s.JIMM.OpenFGAClient.CheckRelation(ctx, tuples[0], false)
	c.Assert(err, qt.IsNil)
	c.Assert(allowed, qt.Equals, false)
	newTuple := tuples[0]
	newTuple.Object = ofganames.ConvertTag(names.NewUserTag("foo2@canonical.com"))
	allowed, err = s.JIMM.OpenFGAClient.CheckRelation(ctx, newTuple, false)
	c.Assert(err, qt.IsNil)
	c.Assert(allowed, qt.Equals, true)
}

func TestGetGroupRolesIntegration(t *testing.T) {
	c := qt.New(t)
	s := SetupGroupsTest(c)

	ctx := c.Context()
	group := s.AddGroup(c, "test-group")
	role := s.AddRole(c, "test-role")
	tuple := openfga.Tuple{
		Object:   ofganames.ConvertTagWithRelation(jimmnames.NewGroupTag(group.UUID), ofganames.MemberRelation),
		Relation: ofganames.AssigneeRelation,
		Target:   ofganames.ConvertTag(jimmnames.NewRoleTag(role.UUID)),
	}
	err := s.JIMM.OpenFGAClient.AddRelation(ctx, tuple)
	c.Assert(err, qt.IsNil)

	params := &resources.GetGroupsItemRolesParams{}
	ctx = rebac_handlers.ContextWithIdentity(ctx, s.AdminUser)
	res, err := s.groupSvc.GetGroupRoles(ctx, group.UUID, params)
	c.Assert(err, qt.IsNil)
	c.Assert(res, qt.Not(qt.IsNil))
	c.Assert(res.Meta.Size, qt.Equals, 1)
	c.Assert(*res.Meta.PageToken, qt.Equals, "")
	c.Assert(res.Next.PageToken, qt.IsNil)
	c.Assert(res.Data, qt.HasLen, 1)
	c.Assert(res.Data[0].Id, qt.Not(qt.IsNil))
	c.Assert(*res.Data[0].Id, qt.Equals, role.UUID)
	c.Assert(res.Data[0].Name, qt.Equals, role.Name)
}

func TestPatchGroupRolesIntegration(t *testing.T) {
	c := qt.New(t)
	s := SetupGroupsTest(c)

	ctx := c.Context()
	group := s.AddGroup(c, "test-group")
	role := s.AddRole(c, "test-role")

	// Assign the role to the group.
	rolePatches := []resources.GroupRolesPatchItem{
		{Role: role.UUID, Op: resources.GroupRolesPatchItemOpAdd},
	}
	ctx = rebac_handlers.ContextWithIdentity(ctx, s.AdminUser)
	res, err := s.groupSvc.PatchGroupRoles(ctx, group.UUID, rolePatches)
	c.Assert(err, qt.IsNil)
	c.Assert(res, qt.Equals, true)

	checkTuple := openfga.Tuple{
		Object:   ofganames.ConvertTagWithRelation(group.ResourceTag(), ofganames.MemberRelation),
		Relation: ofganames.AssigneeRelation,
		Target:   ofganames.ConvertTag(role.ResourceTag()),
	}
	allowed, err := s.JIMM.OpenFGAClient.CheckRelation(ctx, checkTuple, false)
	c.Assert(err, qt.IsNil)
	c.Assert(allowed, qt.Equals, true)

	// Remove the role from the group.
	rolePatches[0].Op = resources.GroupRolesPatchItemOpRemove
	ctx = rebac_handlers.ContextWithIdentity(ctx, s.AdminUser)
	res, err = s.groupSvc.PatchGroupRoles(ctx, group.UUID, rolePatches)
	c.Assert(err, qt.IsNil)
	c.Assert(res, qt.Equals, true)

	allowed, err = s.JIMM.OpenFGAClient.CheckRelation(ctx, checkTuple, false)
	c.Assert(err, qt.IsNil)
	c.Assert(allowed, qt.Equals, false)
}

func TestGetGroupEntitlementsIntegration(t *testing.T) {
	c := qt.New(t)
	s := SetupGroupsTest(c)

	ctx := c.Context()
	group, err := s.JIMM.GroupManager.AddGroup(ctx, s.AdminUser, "test-group")
	c.Assert(err, qt.IsNil)
	tuple := openfga.Tuple{
		Object:   ofganames.ConvertTagWithRelation(jimmnames.NewGroupTag(group.UUID), ofganames.MemberRelation),
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
	c.Assert(err, qt.IsNil)

	ctx = rebac_handlers.ContextWithIdentity(ctx, s.AdminUser)
	emptyPageToken := ""
	req := resources.GetGroupsItemEntitlementsParams{NextPageToken: &emptyPageToken}
	var entitlements []resources.EntityEntitlement
	res, err := s.groupSvc.GetGroupEntitlements(ctx, group.UUID, &req)
	c.Assert(err, qt.IsNil)
	c.Assert(res, qt.Not(qt.IsNil))
	entitlements = append(entitlements, res.Data...)
	c.Assert(entitlements, qt.HasLen, 6)
	modelEntitlementCount := 0
	controllerEntitlementCount := 0
	for _, entitlement := range entitlements {
		c.Assert(entitlement.Entitlement, qt.Equals, ofganames.AdministratorRelation.String())
		c.Assert(entitlement.EntityId, qt.Matches, `test-(model|controller)-\d`)
		switch entitlement.EntityType {
		case openfga.ModelType.String():
			modelEntitlementCount++
		case openfga.ControllerType.String():
			controllerEntitlementCount++
		default:
			c.Logf("Unexpected entitlement found of type %s", entitlement.EntityType)
			c.FailNow()
		}
	}
	c.Assert(modelEntitlementCount, qt.Equals, 3)
	c.Assert(controllerEntitlementCount, qt.Equals, 3)
}

// patchGroupEntitlementTestEnv is used to create entries in JIMM's database.
// The rebacAdminSuite does not spin up a Juju controller so we cannot use
// regular JIMM methods to create resources. It is also necessary to have resources
// present in the database in order for ListRelationshipTuples to work correctly.
const patchGroupEntitlementTestEnv = `clouds:
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

// TestPatchGroupEntitlementsIntegration creates 4 models and verifies that relations from a group to these models can be added/removed.
func TestPatchGroupEntitlementsIntegration(t *testing.T) {
	c := qt.New(t)
	s := SetupGroupsTest(c)

	ctx := c.Context()
	env := jimmtest.ParseEnvironment(c, patchGroupEntitlementTestEnv)
	env.PopulateDB(c, s.JIMM.Database)
	oldModels := []string{env.Models[0].UUID, env.Models[1].UUID}
	newModels := []string{env.Models[2].UUID, env.Models[3].UUID}

	group, err := s.JIMM.GroupManager.AddGroup(ctx, s.AdminUser, "test-group")
	c.Assert(err, qt.IsNil)
	tuple := openfga.Tuple{
		Object:   ofganames.ConvertTagWithRelation(jimmnames.NewGroupTag(group.UUID), ofganames.MemberRelation),
		Relation: ofganames.AdministratorRelation,
	}

	var tuples []openfga.Tuple
	for i := range 2 {
		t := tuple
		t.Target = ofganames.ConvertTag(names.NewModelTag(oldModels[i]))
		tuples = append(tuples, t)
	}
	err = s.JIMM.OpenFGAClient.AddRelation(ctx, tuples...)
	c.Assert(err, qt.IsNil)
	allowed, err := s.JIMM.OpenFGAClient.CheckRelation(ctx, tuples[0], false)
	c.Assert(err, qt.IsNil)
	c.Assert(allowed, qt.Equals, true)
	// Above we have added granted the group with administrator permission to 2 models.
	// Below, we will request those 2 relations to be removed and add 2 different relations.

	entitlementPatches := []resources.GroupEntitlementsPatchItem{
		{Entitlement: resources.EntityEntitlement{
			Entitlement: ofganames.AdministratorRelation.String(),
			EntityId:    newModels[0],
			EntityType:  openfga.ModelType.String(),
		}, Op: resources.GroupEntitlementsPatchItemOpAdd},
		{Entitlement: resources.EntityEntitlement{
			Entitlement: ofganames.AdministratorRelation.String(),
			EntityId:    newModels[1],
			EntityType:  openfga.ModelType.String(),
		}, Op: resources.GroupEntitlementsPatchItemOpAdd},
		{Entitlement: resources.EntityEntitlement{
			Entitlement: ofganames.AdministratorRelation.String(),
			EntityId:    oldModels[0],
			EntityType:  openfga.ModelType.String(),
		}, Op: resources.GroupEntitlementsPatchItemOpRemove},
		{Entitlement: resources.EntityEntitlement{
			Entitlement: ofganames.AdministratorRelation.String(),
			EntityId:    oldModels[1],
			EntityType:  openfga.ModelType.String(),
		}, Op: resources.GroupEntitlementsPatchItemOpRemove},
	}
	ctx = rebac_handlers.ContextWithIdentity(ctx, s.AdminUser)
	res, err := s.groupSvc.PatchGroupEntitlements(ctx, group.UUID, entitlementPatches)
	c.Assert(err, qt.IsNil)
	c.Assert(res, qt.Equals, true)

	for i := range 2 {
		allowed, err = s.JIMM.OpenFGAClient.CheckRelation(ctx, tuples[i], false)
		c.Assert(err, qt.IsNil)
		c.Assert(allowed, qt.Equals, false)
	}
	for i := range 2 {
		newTuple := tuples[0]
		newTuple.Target = ofganames.ConvertTag(names.NewModelTag(newModels[i]))
		allowed, err = s.JIMM.OpenFGAClient.CheckRelation(ctx, newTuple, false)
		c.Assert(err, qt.IsNil)
		c.Assert(allowed, qt.Equals, true)
	}
}
