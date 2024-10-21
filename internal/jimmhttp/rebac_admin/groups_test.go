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
	"github.com/canonical/jimm/v3/internal/jimmhttp/rebac_admin"
	"github.com/canonical/jimm/v3/internal/openfga"
	"github.com/canonical/jimm/v3/internal/testutils/jimmtest"
	"github.com/canonical/jimm/v3/internal/testutils/jimmtest/mocks"
	"github.com/canonical/jimm/v3/pkg/api/params"
)

func TestCreateGroup(t *testing.T) {
	c := qt.New(t)
	var addErr error
	jimm := jimmtest.JIMM{
		GroupService: mocks.GroupService{
			AddGroup_: func(ctx context.Context, user *openfga.User, name string) (*dbmodel.GroupEntry, error) {
				return &dbmodel.GroupEntry{UUID: "test-uuid", Name: name}, addErr
			},
		},
	}
	user := openfga.User{}
	ctx := context.Background()
	ctx = rebac_handlers.ContextWithIdentity(ctx, &user)
	groupSvc := rebac_admin.NewGroupService(&jimm)
	resp, err := groupSvc.CreateGroup(ctx, &resources.Group{Name: "new-group"})
	c.Assert(err, qt.IsNil)
	c.Assert(*resp.Id, qt.Equals, "test-uuid")
	c.Assert(resp.Name, qt.Equals, "new-group")
	addErr = errors.New("foo")
	_, err = groupSvc.CreateGroup(ctx, &resources.Group{Name: "new-group"})
	c.Assert(err, qt.ErrorMatches, "foo")
}

func TestUpdateGroup(t *testing.T) {
	c := qt.New(t)
	groupID := "group-id"
	var renameErr error
	jimm := jimmtest.JIMM{
		GroupService: mocks.GroupService{
			GetGroupByUUID_: func(ctx context.Context, user *openfga.User, uuid string) (*dbmodel.GroupEntry, error) {
				return &dbmodel.GroupEntry{UUID: groupID, Name: "test-group"}, nil
			},
			RenameGroup_: func(ctx context.Context, user *openfga.User, oldName, newName string) error {
				if oldName != "test-group" {
					return errors.New("invalid old group name")
				}
				return renameErr
			},
		},
	}
	user := openfga.User{}
	ctx := context.Background()
	ctx = rebac_handlers.ContextWithIdentity(ctx, &user)
	groupSvc := rebac_admin.NewGroupService(&jimm)
	_, err := groupSvc.UpdateGroup(ctx, &resources.Group{Name: "new-group"})
	c.Assert(err, qt.ErrorMatches, ".*missing group ID")
	resp, err := groupSvc.UpdateGroup(ctx, &resources.Group{Id: &groupID, Name: "new-group"})
	c.Assert(err, qt.IsNil)
	c.Assert(resp, qt.DeepEquals, &resources.Group{Id: &groupID, Name: "new-group"})
	renameErr = errors.New("foo")
	_, err = groupSvc.UpdateGroup(ctx, &resources.Group{Id: &groupID, Name: "new-group"})
	c.Assert(err, qt.ErrorMatches, "foo")
}

func TestListGroups(t *testing.T) {
	c := qt.New(t)
	var listErr error
	returnedGroups := []dbmodel.GroupEntry{
		{Name: "group-1"},
		{Name: "group-2"},
		{Name: "group-3"},
	}
	jimm := jimmtest.JIMM{
		GroupService: mocks.GroupService{
			ListGroups_: func(ctx context.Context, user *openfga.User, filter pagination.LimitOffsetPagination) ([]dbmodel.GroupEntry, error) {
				return returnedGroups, listErr
			},
			CountGroups_: func(ctx context.Context, user *openfga.User) (int, error) {
				return 10, nil
			},
		},
	}
	expected := []resources.Group{}
	id := ""
	for _, group := range returnedGroups {
		expected = append(expected, resources.Group{Name: group.Name, Id: &id})
	}
	user := openfga.User{}
	ctx := context.Background()
	ctx = rebac_handlers.ContextWithIdentity(ctx, &user)
	groupSvc := rebac_admin.NewGroupService(&jimm)
	resp, err := groupSvc.ListGroups(ctx, &resources.GetGroupsParams{})
	c.Assert(err, qt.IsNil)
	c.Assert(resp.Data, qt.DeepEquals, expected)
	c.Assert(*resp.Meta.Page, qt.Equals, 0)
	c.Assert(resp.Meta.Size, qt.Equals, len(expected))
	c.Assert(*resp.Meta.Total, qt.Equals, 10)
	c.Assert(*resp.Next.Page, qt.Equals, 1)
	listErr = errors.New("foo")
	_, err = groupSvc.ListGroups(ctx, &resources.GetGroupsParams{})
	c.Assert(err, qt.ErrorMatches, "foo")
}

func TestDeleteGroup(t *testing.T) {
	c := qt.New(t)
	var deleteErr error
	jimm := jimmtest.JIMM{
		GroupService: mocks.GroupService{
			GetGroupByUUID_: func(ctx context.Context, user *openfga.User, uuid string) (*dbmodel.GroupEntry, error) {
				return &dbmodel.GroupEntry{UUID: uuid, Name: "test-group"}, nil
			},
			RemoveGroup_: func(ctx context.Context, user *openfga.User, name string) error {
				if name != "test-group" {
					return errors.New("invalid name provided")
				}
				return deleteErr
			},
		},
	}
	user := openfga.User{}
	ctx := context.Background()
	ctx = rebac_handlers.ContextWithIdentity(ctx, &user)
	groupSvc := rebac_admin.NewGroupService(&jimm)
	res, err := groupSvc.DeleteGroup(ctx, "group-id")
	c.Assert(res, qt.IsTrue)
	c.Assert(err, qt.IsNil)
	deleteErr = errors.New("foo")
	_, err = groupSvc.DeleteGroup(ctx, "group-id")
	c.Assert(err, qt.ErrorMatches, "foo")
}

func TestGetGroupIdentities(t *testing.T) {
	c := qt.New(t)
	var listTuplesErr error
	var getGroupErr error
	var continuationToken string
	testTuple := openfga.Tuple{
		Object:   &ofga.Entity{Kind: "user", ID: "foo"},
		Relation: ofga.Relation("member"),
		Target:   &ofga.Entity{Kind: "group", ID: "my-group"},
	}
	jimm := jimmtest.JIMM{
		GroupService: mocks.GroupService{
			GetGroupByUUID_: func(ctx context.Context, user *openfga.User, uuid string) (*dbmodel.GroupEntry, error) {
				return nil, getGroupErr
			},
		},
		RelationService: mocks.RelationService{
			ListRelationshipTuples_: func(ctx context.Context, user *openfga.User, tuple params.RelationshipTuple, pageSize int32, ct string) ([]openfga.Tuple, string, error) {
				return []openfga.Tuple{testTuple}, continuationToken, listTuplesErr
			},
		},
	}
	user := openfga.User{}
	ctx := context.Background()
	ctx = rebac_handlers.ContextWithIdentity(ctx, &user)
	groupSvc := rebac_admin.NewGroupService(&jimm)

	_, err := groupSvc.GetGroupIdentities(ctx, "invalid-group-id", &resources.GetGroupsItemIdentitiesParams{})
	c.Assert(err, qt.ErrorMatches, ".*invalid group ID")

	newUUID := uuid.New()
	getGroupErr = errors.New("group doesn't exist")
	_, err = groupSvc.GetGroupIdentities(ctx, newUUID.String(), &resources.GetGroupsItemIdentitiesParams{})
	c.Assert(err, qt.ErrorMatches, ".*group doesn't exist")
	getGroupErr = nil

	continuationToken = "continuation-token"
	res, err := groupSvc.GetGroupIdentities(ctx, newUUID.String(), &resources.GetGroupsItemIdentitiesParams{})
	c.Assert(err, qt.IsNil)
	c.Assert(res, qt.IsNotNil)
	c.Assert(res.Data, qt.HasLen, 1)
	c.Assert(*res.Next.PageToken, qt.Equals, "continuation-token")

	continuationToken = ""
	res, err = groupSvc.GetGroupIdentities(ctx, newUUID.String(), &resources.GetGroupsItemIdentitiesParams{})
	c.Assert(err, qt.IsNil)
	c.Assert(res, qt.IsNotNil)
	c.Assert(res.Next.PageToken, qt.IsNil)

	listTuplesErr = errors.New("foo")
	_, err = groupSvc.GetGroupIdentities(ctx, newUUID.String(), &resources.GetGroupsItemIdentitiesParams{})
	c.Assert(err, qt.ErrorMatches, "foo")
}

func TestPatchGroupIdentities(t *testing.T) {
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
	groupSvc := rebac_admin.NewGroupService(&jimm)

	_, err := groupSvc.PatchGroupIdentities(ctx, "invalid-group-id", nil)
	c.Assert(err, qt.ErrorMatches, ".* invalid group ID")

	newUUID := uuid.New()
	operations := []resources.GroupIdentitiesPatchItem{
		{Identity: "foo@canonical.com", Op: resources.GroupIdentitiesPatchItemOpAdd},
		{Identity: "bar@canonical.com", Op: resources.GroupIdentitiesPatchItemOpRemove},
	}
	res, err := groupSvc.PatchGroupIdentities(ctx, newUUID.String(), operations)
	c.Assert(err, qt.IsNil)
	c.Assert(res, qt.IsTrue)

	operationsWithInvalidIdentity := []resources.GroupIdentitiesPatchItem{
		{Identity: "foo_", Op: resources.GroupIdentitiesPatchItemOpAdd},
	}
	_, err = groupSvc.PatchGroupIdentities(ctx, newUUID.String(), operationsWithInvalidIdentity)
	c.Assert(err, qt.ErrorMatches, ".*invalid identity.*")

	patchTuplesErr = errors.New("foo")
	_, err = groupSvc.PatchGroupIdentities(ctx, newUUID.String(), operations)
	c.Assert(err, qt.ErrorMatches, "foo")
}

func TestGetGroupEntitlements(t *testing.T) {
	c := qt.New(t)
	var listRelationsErr error
	var continuationToken string
	testTuple := openfga.Tuple{
		Object:   &ofga.Entity{Kind: "user", ID: "foo"},
		Relation: ofga.Relation("member"),
		Target:   &ofga.Entity{Kind: "group", ID: "my-group"},
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
	groupSvc := rebac_admin.NewGroupService(&jimm)

	_, err := groupSvc.GetGroupEntitlements(ctx, "invalid-group-id", nil)
	c.Assert(err, qt.ErrorMatches, ".* invalid group ID")

	continuationToken = "random-token"
	res, err := groupSvc.GetGroupEntitlements(ctx, uuid.New().String(), &resources.GetGroupsItemEntitlementsParams{})
	c.Assert(err, qt.IsNil)
	c.Assert(res, qt.IsNotNil)
	c.Assert(res.Data, qt.HasLen, 1)
	c.Assert(*res.Next.PageToken, qt.Equals, "random-token")

	continuationToken = ""
	res, err = groupSvc.GetGroupEntitlements(ctx, uuid.New().String(), &resources.GetGroupsItemEntitlementsParams{})
	c.Assert(err, qt.IsNil)
	c.Assert(res, qt.IsNotNil)
	c.Assert(res.Next.PageToken, qt.IsNil)

	nextToken := "some-token"
	res, err = groupSvc.GetGroupEntitlements(ctx, uuid.New().String(), &resources.GetGroupsItemEntitlementsParams{NextToken: &nextToken})
	c.Assert(err, qt.IsNil)
	c.Assert(res, qt.IsNotNil)

	listRelationsErr = errors.New("foo")
	_, err = groupSvc.GetGroupEntitlements(ctx, uuid.New().String(), &resources.GetGroupsItemEntitlementsParams{})
	c.Assert(err, qt.ErrorMatches, "foo")
}

func TestPatchGroupEntitlements(t *testing.T) {
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
	groupSvc := rebac_admin.NewGroupService(&jimm)

	_, err := groupSvc.PatchGroupEntitlements(ctx, "invalid-group-id", nil)
	c.Assert(err, qt.ErrorMatches, ".* invalid group ID")

	newUUID := uuid.New()
	operations := []resources.GroupEntitlementsPatchItem{
		{Entitlement: resources.EntityEntitlement{
			Entitlement: "administrator",
			EntityId:    newUUID.String(),
			EntityType:  "model",
		}, Op: resources.GroupEntitlementsPatchItemOpAdd},
		{Entitlement: resources.EntityEntitlement{
			Entitlement: "administrator",
			EntityId:    newUUID.String(),
			EntityType:  "model",
		}, Op: resources.GroupEntitlementsPatchItemOpRemove},
	}
	res, err := groupSvc.PatchGroupEntitlements(ctx, newUUID.String(), operations)
	c.Assert(err, qt.IsNil)
	c.Assert(res, qt.IsTrue)

	operationsWithInvalidTag := []resources.GroupEntitlementsPatchItem{
		{Entitlement: resources.EntityEntitlement{
			Entitlement: "administrator",
			EntityId:    "foo",
			EntityType:  "invalidType",
		}, Op: resources.GroupEntitlementsPatchItemOpAdd},
		{Entitlement: resources.EntityEntitlement{
			Entitlement: "administrator",
			EntityId:    "foo1",
			EntityType:  "invalidType2",
		}, Op: resources.GroupEntitlementsPatchItemOpAdd},
	}
	_, err = groupSvc.PatchGroupEntitlements(ctx, newUUID.String(), operationsWithInvalidTag)
	c.Assert(err, qt.ErrorMatches, `\"invalidType-foo\" is not a valid tag\n\"invalidType2-foo1\" is not a valid tag`)

	patchTuplesErr = errors.New("foo")
	_, err = groupSvc.PatchGroupEntitlements(ctx, newUUID.String(), operations)
	c.Assert(err, qt.ErrorMatches, "foo")
}
