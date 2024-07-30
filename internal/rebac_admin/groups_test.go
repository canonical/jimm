// Copyright 2024 Canonical Ltd.

package rebac_admin_test

import (
	"context"
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
	jimm := jimmtest.JIMM{
		GroupService: jimmtest.GroupService{
			AddGroup_: func(ctx context.Context, user *openfga.User, name string) (*dbmodel.GroupEntry, error) {
				return &dbmodel.GroupEntry{UUID: "test-uuid", Name: name}, nil
			},
		},
	}
	user := openfga.User{}
	ctx := context.Background()
	ctx = rebac_handlers.ContextWithIdentity(ctx, &user)
	groupSvc := rebac_admin.NewGroupService(&jimm)
	resp, err := groupSvc.CreateGroup(ctx, &resources.Group{Name: "new-group"})
	c.Assert(err, qt.IsNil)
	c.Assert(resp, qt.IsNil)
}

func TestUpdateGroup(t *testing.T) {
	c := qt.New(t)
	groupID := "group-id"
	jimm := jimmtest.JIMM{
		GroupService: jimmtest.GroupService{
			GetGroupByID_: func(ctx context.Context, user *openfga.User, uuid string) (dbmodel.GroupEntry, error) {
				return dbmodel.GroupEntry{UUID: groupID}, nil
			},
			RenameGroup_: func(ctx context.Context, user *openfga.User, oldName, newName string) error {
				return nil
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
}

func TestListGroups(t *testing.T) {
	c := qt.New(t)
	returnedGroups := []dbmodel.GroupEntry{
		{Name: "group-1"},
		{Name: "group-2"},
		{Name: "group-3"},
	}
	jimm := jimmtest.JIMM{
		GroupService: jimmtest.GroupService{
			ListGroups_: func(ctx context.Context, user *openfga.User, filter pagination.LimitOffsetPagination) ([]dbmodel.GroupEntry, error) {
				return returnedGroups, nil
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
}

func TestDeleteGroup(t *testing.T) {
	c := qt.New(t)
	jimm := jimmtest.JIMM{
		GroupService: jimmtest.GroupService{
			RemoveGroup_: func(ctx context.Context, user *openfga.User, name string) error {
				return nil
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
}
