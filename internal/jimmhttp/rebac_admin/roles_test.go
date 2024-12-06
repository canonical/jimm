// Copyright 2024 Canonical.

package rebac_admin_test

import (
	"context"
	"errors"
	"testing"

	"github.com/canonical/ofga"
	rebac_handlers "github.com/canonical/rebac-admin-ui-handlers/v1"
	"github.com/canonical/rebac-admin-ui-handlers/v1/resources"
	qt "github.com/frankban/quicktest"
	"github.com/google/uuid"

	"github.com/canonical/jimm/v3/internal/common/pagination"
	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/jimm"
	"github.com/canonical/jimm/v3/internal/jimmhttp/rebac_admin"
	"github.com/canonical/jimm/v3/internal/openfga"
	"github.com/canonical/jimm/v3/internal/testutils/jimmtest"
	"github.com/canonical/jimm/v3/internal/testutils/jimmtest/mocks"
	"github.com/canonical/jimm/v3/pkg/api/params"
)

func TestCreateRole(t *testing.T) {
	c := qt.New(t)
	var addErr error
	roleManager := mocks.RoleManager{
		AddRole_: func(ctx context.Context, user *openfga.User, name string) (*dbmodel.RoleEntry, error) {
			return &dbmodel.RoleEntry{UUID: "test-uuid", Name: name}, addErr
		},
	}
	jimm := jimmtest.JIMM{
		RoleManager_: func() jimm.RoleManager {
			return roleManager
		},
	}
	user := openfga.User{}
	ctx := context.Background()
	ctx = rebac_handlers.ContextWithIdentity(ctx, &user)
	roleSvc := rebac_admin.NewRoleService(&jimm)
	resp, err := roleSvc.CreateRole(ctx, &resources.Role{Name: "new-role"})
	c.Assert(err, qt.IsNil)
	c.Assert(*resp.Id, qt.Equals, "test-uuid")
	c.Assert(resp.Name, qt.Equals, "new-role")
	addErr = errors.New("foo")
	_, err = roleSvc.CreateRole(ctx, &resources.Role{Name: "new-role"})
	c.Assert(err, qt.ErrorMatches, "foo")
}

func TestUpdateRole(t *testing.T) {
	c := qt.New(t)
	roleID := "role-id"
	var renameErr error
	roleManager := mocks.RoleManager{
		GetRoleByUUID_: func(ctx context.Context, user *openfga.User, uuid string) (*dbmodel.RoleEntry, error) {
			return &dbmodel.RoleEntry{UUID: roleID, Name: "test-role"}, nil
		},
		RenameRole_: func(ctx context.Context, user *openfga.User, oldName, newName string) error {
			if oldName != "test-role" {
				return errors.New("invalid old role name")
			}
			return renameErr
		},
	}
	jimm := jimmtest.JIMM{
		RoleManager_: func() jimm.RoleManager {
			return roleManager
		},
	}
	user := openfga.User{}
	ctx := context.Background()
	ctx = rebac_handlers.ContextWithIdentity(ctx, &user)
	roleSvc := rebac_admin.NewRoleService(&jimm)
	_, err := roleSvc.UpdateRole(ctx, &resources.Role{Name: "new-role"})
	c.Assert(err, qt.ErrorMatches, ".*missing role ID")
	resp, err := roleSvc.UpdateRole(ctx, &resources.Role{Id: &roleID, Name: "new-role"})
	c.Assert(err, qt.IsNil)
	c.Assert(resp, qt.DeepEquals, &resources.Role{Id: &roleID, Name: "new-role"})
	renameErr = errors.New("foo")
	_, err = roleSvc.UpdateRole(ctx, &resources.Role{Id: &roleID, Name: "new-role"})
	c.Assert(err, qt.ErrorMatches, "foo")
}

func TestListRoles(t *testing.T) {
	c := qt.New(t)
	var listErr error
	returnedRoles := []dbmodel.RoleEntry{
		{Name: "role-1"},
		{Name: "role-2"},
		{Name: "role-3"},
	}
	roleManager := mocks.RoleManager{
		ListRoles_: func(ctx context.Context, user *openfga.User, pagination pagination.LimitOffsetPagination, match string) ([]dbmodel.RoleEntry, error) {
			return returnedRoles, listErr
		},
		CountRoles_: func(ctx context.Context, user *openfga.User) (int, error) {
			return 10, nil
		},
	}
	jimm := jimmtest.JIMM{
		RoleManager_: func() jimm.RoleManager {
			return roleManager
		},
	}
	expected := []resources.Role{}
	id := ""
	for _, role := range returnedRoles {
		expected = append(expected, resources.Role{Name: role.Name, Id: &id})
	}
	user := openfga.User{}
	ctx := context.Background()
	ctx = rebac_handlers.ContextWithIdentity(ctx, &user)
	roleSvc := rebac_admin.NewRoleService(&jimm)
	resp, err := roleSvc.ListRoles(ctx, &resources.GetRolesParams{})
	c.Assert(err, qt.IsNil)
	c.Assert(resp.Data, qt.DeepEquals, expected)
	c.Assert(*resp.Meta.Page, qt.Equals, 0)
	c.Assert(resp.Meta.Size, qt.Equals, len(expected))
	c.Assert(*resp.Meta.Total, qt.Equals, 10)
	c.Assert(*resp.Next.Page, qt.Equals, 1)
	listErr = errors.New("foo")
	_, err = roleSvc.ListRoles(ctx, &resources.GetRolesParams{})
	c.Assert(err, qt.ErrorMatches, "foo")
}

func TestDeleteRole(t *testing.T) {
	c := qt.New(t)
	var deleteErr error
	roleManager := mocks.RoleManager{
		GetRoleByUUID_: func(ctx context.Context, user *openfga.User, uuid string) (*dbmodel.RoleEntry, error) {
			return &dbmodel.RoleEntry{UUID: uuid, Name: "test-role"}, nil
		},
		RemoveRole_: func(ctx context.Context, user *openfga.User, name string) error {
			if name != "test-role" {
				return errors.New("invalid name provided")
			}
			return deleteErr
		},
	}
	jimm := jimmtest.JIMM{
		RoleManager_: func() jimm.RoleManager {
			return roleManager
		},
	}
	user := openfga.User{}
	ctx := context.Background()
	ctx = rebac_handlers.ContextWithIdentity(ctx, &user)
	roleSvc := rebac_admin.NewRoleService(&jimm)
	res, err := roleSvc.DeleteRole(ctx, "role-id")
	c.Assert(res, qt.IsTrue)
	c.Assert(err, qt.IsNil)
	deleteErr = errors.New("foo")
	_, err = roleSvc.DeleteRole(ctx, "role-id")
	c.Assert(err, qt.ErrorMatches, "foo")
}

func TestGetRoleEntitlements(t *testing.T) {
	c := qt.New(t)
	var listRelationsErr error
	var continuationToken string
	testTuple := openfga.Tuple{
		Object:   &ofga.Entity{Kind: "user", ID: "foo"},
		Relation: ofga.Relation("member"),
		Target:   &ofga.Entity{Kind: "role", ID: "my-role"},
	}
	jimm := jimmtest.JIMM{
		RelationService: mocks.RelationService{
			ListObjectRelations_: func(ctx context.Context, user *openfga.User, object string, pageSize int32, ct pagination.EntitlementToken) ([]openfga.Tuple, pagination.EntitlementToken, error) {
				return []openfga.Tuple{testTuple}, pagination.NewEntitlementToken(continuationToken), listRelationsErr
			},
		},
	}
	user := openfga.User{}
	ctx := context.Background()
	ctx = rebac_handlers.ContextWithIdentity(ctx, &user)
	roleSvc := rebac_admin.NewRoleService(&jimm)

	_, err := roleSvc.GetRoleEntitlements(ctx, "invalid-role-id", nil)
	c.Assert(err, qt.ErrorMatches, ".* invalid role ID")

	continuationToken = "random-token"
	res, err := roleSvc.GetRoleEntitlements(ctx, uuid.New().String(), &resources.GetRolesItemEntitlementsParams{})
	c.Assert(err, qt.IsNil)
	c.Assert(res, qt.IsNotNil)
	c.Assert(res.Data, qt.HasLen, 1)
	c.Assert(*res.Next.PageToken, qt.Equals, "random-token")

	continuationToken = ""
	res, err = roleSvc.GetRoleEntitlements(ctx, uuid.New().String(), &resources.GetRolesItemEntitlementsParams{})
	c.Assert(err, qt.IsNil)
	c.Assert(res, qt.IsNotNil)
	c.Assert(res.Next.PageToken, qt.IsNil)

	nextToken := "some-token"
	res, err = roleSvc.GetRoleEntitlements(ctx, uuid.New().String(), &resources.GetRolesItemEntitlementsParams{NextToken: &nextToken})
	c.Assert(err, qt.IsNil)
	c.Assert(res, qt.IsNotNil)

	listRelationsErr = errors.New("foo")
	_, err = roleSvc.GetRoleEntitlements(ctx, uuid.New().String(), &resources.GetRolesItemEntitlementsParams{})
	c.Assert(err, qt.ErrorMatches, "foo")
}

func TestPatchRoleEntitlements(t *testing.T) {
	c := qt.New(t)
	var patchTuplesErr error
	jimm := jimmtest.JIMM{
		RelationService: mocks.RelationService{
			AddRelation_: func(ctx context.Context, user *openfga.User, tuples []params.RelationshipTuple) error {
				return patchTuplesErr
			},
			RemoveRelation_: func(ctx context.Context, user *openfga.User, tuples []params.RelationshipTuple) error {
				return patchTuplesErr
			},
		},
	}
	user := openfga.User{}
	ctx := context.Background()
	ctx = rebac_handlers.ContextWithIdentity(ctx, &user)
	roleSvc := rebac_admin.NewRoleService(&jimm)

	_, err := roleSvc.PatchRoleEntitlements(ctx, "invalid-role-id", nil)
	c.Assert(err, qt.ErrorMatches, ".* invalid role ID")

	newUUID := uuid.New()
	operations := []resources.RoleEntitlementsPatchItem{
		{Entitlement: resources.EntityEntitlement{
			Entitlement: "administrator",
			EntityId:    newUUID.String(),
			EntityType:  "model",
		}, Op: resources.Add},
		{Entitlement: resources.EntityEntitlement{
			Entitlement: "administrator",
			EntityId:    newUUID.String(),
			EntityType:  "model",
		}, Op: resources.Remove},
	}
	res, err := roleSvc.PatchRoleEntitlements(ctx, newUUID.String(), operations)
	c.Assert(err, qt.IsNil)
	c.Assert(res, qt.IsTrue)

	operationsWithInvalidTag := []resources.RoleEntitlementsPatchItem{
		{Entitlement: resources.EntityEntitlement{
			Entitlement: "administrator",
			EntityId:    "foo",
			EntityType:  "invalidType",
		}, Op: resources.Add},
		{Entitlement: resources.EntityEntitlement{
			Entitlement: "administrator",
			EntityId:    "foo1",
			EntityType:  "invalidType2",
		}, Op: resources.Add},
	}
	_, err = roleSvc.PatchRoleEntitlements(ctx, newUUID.String(), operationsWithInvalidTag)
	c.Assert(err, qt.ErrorMatches, `\"invalidType-foo\" is not a valid tag\n\"invalidType2-foo1\" is not a valid tag`)

	patchTuplesErr = errors.New("foo")
	_, err = roleSvc.PatchRoleEntitlements(ctx, newUUID.String(), operations)
	c.Assert(err, qt.ErrorMatches, "foo")
}
