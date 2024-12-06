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
	jimmnames "github.com/canonical/jimm/v3/pkg/names"
)

type roleSuite struct {
	jimmtest.JIMMSuite
	roleSvc *rebac_admin.RolesService
}

func (s *roleSuite) SetUpTest(c *gc.C) {
	s.JIMMSuite.SetUpTest(c)
	s.roleSvc = rebac_admin.NewRoleService(s.JIMM)
}

var _ = gc.Suite(&roleSuite{})

func (s roleSuite) TestListRolesWithFilterIntegration(c *gc.C) {
	ctx := context.Background()
	for i := range 10 {
		_, err := s.JIMM.RoleManager().AddRole(ctx, s.AdminUser, fmt.Sprintf("test-role-filter-%d", i))
		c.Assert(err, gc.IsNil)
	}

	ctx = rebac_handlers.ContextWithIdentity(ctx, s.AdminUser)
	pageSize := 5
	page := 0
	params := &resources.GetRolesParams{Size: &pageSize, Page: &page}
	res, err := s.roleSvc.ListRoles(ctx, params)
	c.Assert(err, gc.IsNil)
	c.Assert(res, gc.Not(gc.IsNil))
	c.Assert(res.Meta.Size, gc.Equals, 5)

	match := "role-filter-1"
	params.Filter = &match
	res, err = s.roleSvc.ListRoles(ctx, params)
	c.Assert(err, gc.IsNil)
	c.Assert(res, gc.Not(gc.IsNil))
	c.Assert(len(res.Data), gc.Equals, 1)

	match = "role"
	params.Filter = &match
	res, err = s.roleSvc.ListRoles(ctx, params)
	c.Assert(err, gc.IsNil)
	c.Assert(res, gc.Not(gc.IsNil))
	c.Assert(len(res.Data), gc.Equals, pageSize)
}

func (s roleSuite) TestGetRoleEntitlementsIntegration(c *gc.C) {
	ctx := context.Background()
	role, err := s.JIMM.RoleManager().AddRole(ctx, s.AdminUser, "test-role")
	c.Assert(err, gc.IsNil)
	tuple := openfga.Tuple{
		Object:   ofganames.ConvertTagWithRelation(jimmnames.NewRoleTag(role.UUID), ofganames.AssigneeRelation),
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
	req := resources.GetRolesItemEntitlementsParams{NextPageToken: &emptyPageToken}
	var entitlements []resources.EntityEntitlement
	res, err := s.roleSvc.GetRoleEntitlements(ctx, role.UUID, &req)
	c.Assert(err, gc.IsNil)
	c.Assert(res, gc.Not(gc.IsNil))
	entitlements = append(entitlements, res.Data...)
	c.Assert(entitlements, gc.HasLen, 6)
	modelEntitlementCount := 0
	controllerEntitlementCount := 0
	for _, entitlement := range entitlements {
		c.Assert(entitlement.Entitlement, gc.Equals, ofganames.AdministratorRelation.String())
		c.Assert(entitlement.EntityId, gc.Matches, `test-(model|controller)-\d`)
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
	c.Assert(modelEntitlementCount, gc.Equals, 3)
	c.Assert(controllerEntitlementCount, gc.Equals, 3)
}

// patchRoleEntitlementTestEnv is used to create entries in JIMM's database.
// The roleSuite does not spin up a Juju controller so we cannot use
// regular JIMM methods to create resources. It is also necessary to have resources
// present in the database in order for ListRelationshipTuples to work correctly.
const patchRoleEntitlementTestEnv = `clouds:
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

// TestPatchRoleEntitlementsIntegration creates 4 models and verifies that relations from a role to these models can be added/removed.
func (s roleSuite) TestPatchRoleEntitlementsIntegration(c *gc.C) {
	ctx := context.Background()
	tester := jimmtest.GocheckTester{C: c}
	env := jimmtest.ParseEnvironment(tester, patchRoleEntitlementTestEnv)
	env.PopulateDB(tester, s.JIMM.Database)
	oldModels := []string{env.Models[0].UUID, env.Models[1].UUID}
	newModels := []string{env.Models[2].UUID, env.Models[3].UUID}

	role, err := s.JIMM.RoleManager().AddRole(ctx, s.AdminUser, "test-role")
	c.Assert(err, gc.IsNil)
	tuple := openfga.Tuple{
		Object:   ofganames.ConvertTagWithRelation(jimmnames.NewRoleTag(role.UUID), ofganames.AssigneeRelation),
		Relation: ofganames.AdministratorRelation,
	}

	var tuples []openfga.Tuple
	for i := range 2 {
		t := tuple
		t.Target = ofganames.ConvertTag(names.NewModelTag(oldModels[i]))
		tuples = append(tuples, t)
	}
	err = s.JIMM.OpenFGAClient.AddRelation(ctx, tuples...)
	c.Assert(err, gc.IsNil)
	allowed, err := s.JIMM.OpenFGAClient.CheckRelation(ctx, tuples[0], false)
	c.Assert(err, gc.IsNil)
	c.Assert(allowed, gc.Equals, true)
	// Above we have added granted the role with administrator permission to 2 models.
	// Below, we will request those 2 relations to be removed and add 2 different relations.

	entitlementPatches := []resources.RoleEntitlementsPatchItem{
		{Entitlement: resources.EntityEntitlement{
			Entitlement: ofganames.AdministratorRelation.String(),
			EntityId:    newModels[0],
			EntityType:  openfga.ModelType.String(),
		}, Op: resources.Add},
		{Entitlement: resources.EntityEntitlement{
			Entitlement: ofganames.AdministratorRelation.String(),
			EntityId:    newModels[1],
			EntityType:  openfga.ModelType.String(),
		}, Op: resources.Add},
		{Entitlement: resources.EntityEntitlement{
			Entitlement: ofganames.AdministratorRelation.String(),
			EntityId:    oldModels[0],
			EntityType:  openfga.ModelType.String(),
		}, Op: resources.Remove},
		{Entitlement: resources.EntityEntitlement{
			Entitlement: ofganames.AdministratorRelation.String(),
			EntityId:    oldModels[1],
			EntityType:  openfga.ModelType.String(),
		}, Op: resources.Remove},
	}
	ctx = rebac_handlers.ContextWithIdentity(ctx, s.AdminUser)
	res, err := s.roleSvc.PatchRoleEntitlements(ctx, role.UUID, entitlementPatches)
	c.Assert(err, gc.IsNil)
	c.Assert(res, gc.Equals, true)

	for i := range 2 {
		allowed, err = s.JIMM.OpenFGAClient.CheckRelation(ctx, tuples[i], false)
		c.Assert(err, gc.IsNil)
		c.Assert(allowed, gc.Equals, false)
	}
	for i := range 2 {
		newTuple := tuples[0]
		newTuple.Target = ofganames.ConvertTag(names.NewModelTag(newModels[i]))
		allowed, err = s.JIMM.OpenFGAClient.CheckRelation(ctx, newTuple, false)
		c.Assert(err, gc.IsNil)
		c.Assert(allowed, gc.Equals, true)
	}
}
