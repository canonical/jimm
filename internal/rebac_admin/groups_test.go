// Copyright 2024 Canonical Ltd.

package rebac_admin_test

import (
	"context"
	"errors"
	"testing"

	"github.com/canonical/jimm/v3/internal/common/pagination"
	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/jimmtest"
	"github.com/canonical/jimm/v3/internal/openfga"
	"github.com/canonical/jimm/v3/internal/rebac_admin"
	rebac_handlers "github.com/canonical/rebac-admin-ui-handlers/v1"
	"github.com/canonical/rebac-admin-ui-handlers/v1/resources"
	qt "github.com/frankban/quicktest"
)

func TestCreateGroup(t *testing.T) {
	c := qt.New(t)
	var addErr error
	jimm := jimmtest.JIMM{
		GroupService: jimmtest.GroupService{
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
		GroupService: jimmtest.GroupService{
			GetGroupByID_: func(ctx context.Context, user *openfga.User, uuid string) (dbmodel.GroupEntry, error) {
				return dbmodel.GroupEntry{UUID: groupID, Name: "test-group"}, nil
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
	c.Assert(err, qt.ErrorMatches, "missing group ID")
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
		GroupService: jimmtest.GroupService{
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
		GroupService: jimmtest.GroupService{
			GetGroupByID_: func(ctx context.Context, user *openfga.User, uuid string) (dbmodel.GroupEntry, error) {
				return dbmodel.GroupEntry{UUID: uuid, Name: "test-group"}, nil
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
