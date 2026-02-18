// Copyright 2025 Canonical.

// The group package provides business logic for handling group related methods..
package group

import (
	"context"

	"github.com/canonical/jimm/v3/internal/common/pagination"
	"github.com/canonical/jimm/v3/internal/db"
	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/openfga"
)

// GroupManager provides a means to manage groups within JIMM.
type GroupManager struct {
	store   *db.Database
	authSvc *openfga.OFGAClient
}

// NewGroupManager returns a new group manager that provides group
// creation, modification, and removal.
func NewGroupManager(store *db.Database, authSvc *openfga.OFGAClient) (*GroupManager, error) {
	if store == nil {
		return nil, errors.E("group store cannot be nil")
	}
	if authSvc == nil {
		return nil, errors.E("group authorisation service cannot be nil")
	}
	return &GroupManager{store, authSvc}, nil
}

// AddGroup creates a group within JIMMs DB for reference by OpenFGA.
func (j *GroupManager) AddGroup(ctx context.Context, user *openfga.User, name string) (*dbmodel.GroupEntry, error) {

	if !user.JimmAdmin {
		return nil, errors.E(errors.CodeUnauthorized, "unauthorized")
	}

	ge, err := j.store.AddGroup(ctx, name)
	if err != nil {
		return nil, errors.E(err)
	}
	return ge, nil
}

// CountGroups returns the number of groups that exist.
func (j *GroupManager) CountGroups(ctx context.Context, user *openfga.User) (int, error) {

	if !user.JimmAdmin {
		return 0, errors.E(errors.CodeUnauthorized, "unauthorized")
	}
	count, err := j.store.CountGroups(ctx)
	if err != nil {
		return 0, errors.E(err)
	}
	return count, nil
}

// getGroup returns a group based on the provided UUID or name.
func (j *GroupManager) getGroup(ctx context.Context, user *openfga.User, group *dbmodel.GroupEntry) (*dbmodel.GroupEntry, error) {

	if !user.JimmAdmin {
		return nil, errors.E(errors.CodeUnauthorized, "unauthorized")
	}
	if err := j.store.GetGroup(ctx, group); err != nil {
		return nil, errors.E(err)
	}
	return group, nil
}

// GetGroupByUUID returns a group based on the provided UUID.
func (j *GroupManager) GetGroupByUUID(ctx context.Context, user *openfga.User, uuid string) (*dbmodel.GroupEntry, error) {
	return j.getGroup(ctx, user, &dbmodel.GroupEntry{UUID: uuid})
}

// GetGroupByName returns a group based on the provided name.
func (j *GroupManager) GetGroupByName(ctx context.Context, user *openfga.User, name string) (*dbmodel.GroupEntry, error) {
	return j.getGroup(ctx, user, &dbmodel.GroupEntry{Name: name})
}

// RenameGroup renames a group in JIMM's DB.
func (j *GroupManager) RenameGroup(ctx context.Context, user *openfga.User, oldName, newName string) error {

	if !user.JimmAdmin {
		return errors.E(errors.CodeUnauthorized, "unauthorized")
	}

	group := &dbmodel.GroupEntry{
		Name: oldName,
	}

	err := j.store.Transaction(func(d *db.Database) error {
		err := d.GetGroup(ctx, group)
		if err != nil {
			return err
		}

		if err := d.UpdateGroupName(ctx, group.UUID, newName); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return errors.E(err)
	}

	return nil
}

// RemoveGroup removes a group within JIMMs DB for reference by OpenFGA.
func (j *GroupManager) RemoveGroup(ctx context.Context, user *openfga.User, name string) error {

	if !user.JimmAdmin {
		return errors.E(errors.CodeUnauthorized, "unauthorized")
	}

	group := &dbmodel.GroupEntry{
		Name: name,
	}
	err := j.store.Transaction(func(d *db.Database) error {
		err := j.store.GetGroup(ctx, group)
		if err != nil {
			return err
		}
		if err := j.store.RemoveGroup(ctx, group); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return errors.E(err)
	}

	err = j.authSvc.RemoveGroup(ctx, group.ResourceTag())
	if err != nil {
		return errors.E(err)
	}
	return nil
}

// ListGroups returns a list of groups known to JIMM.
// `match` will filter the list fuzzy matching group's name or uuid.
func (j *GroupManager) ListGroups(ctx context.Context, user *openfga.User, pagination pagination.LimitOffsetPagination, match string) ([]dbmodel.GroupEntry, error) {

	if !user.JimmAdmin {
		return nil, errors.E(errors.CodeUnauthorized, "unauthorized")
	}

	groups, err := j.store.ListGroups(ctx, pagination.Limit(), pagination.Offset(), match)
	if err != nil {
		return nil, errors.E(err)
	}
	return groups, nil
}
