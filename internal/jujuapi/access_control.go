package jujuapi

import (
	"context"

	apiparams "github.com/CanonicalLtd/jimm/api/params"
	"github.com/CanonicalLtd/jimm/internal/dbmodel"
	"github.com/CanonicalLtd/jimm/internal/errors"
	"github.com/juju/zaputil"
	"github.com/juju/zaputil/zapctx"
)

// access_control contains the primary RPC commands for handling ReBAC within JIMM via the JIMM facade itself.

// AddGroup creates a group within JIMMs DB for reference by OpenFGA.
func (r *controllerRoot) AddGroup(ctx context.Context, req apiparams.AddGroupRequest) error {
	const op = errors.Op("jujuapi.AddGroup")
	if r.user.ControllerAccess != "superuser" {
		return errors.E(op, errors.CodeUnauthorized, "unauthorized")
	}

	if err := r.jimm.Database.AddGroup(ctx, req.Name); err != nil {
		zapctx.Error(ctx, "failed to add group", zaputil.Error(err))
		return errors.E(op, err)
	}
	return nil
}

// RenameGroup renames a group within JIMMs DB for reference by OpenFGA.
func (r *controllerRoot) RenameGroup(ctx context.Context, req apiparams.RenameGroupRequest) error {
	const op = errors.Op("jujuapi.RenameGroup")
	if r.user.ControllerAccess != "superuser" {
		return errors.E(op, errors.CodeUnauthorized, "unauthorized")
	}

	group, err := r.jimm.Database.GetGroup(ctx, req.Name)
	if err != nil {
		return errors.E(op, err)
	}
	group.Name = req.NewName

	if err := r.jimm.Database.UpdateGroup(ctx, group); err != nil {
		zapctx.Error(ctx, "failed to rename group", zaputil.Error(err))
		return errors.E(op, err)
	}
	return nil
}

// RemoveGroup removes a group within JIMMs DB for reference by OpenFGA.
func (r *controllerRoot) RemoveGroup(ctx context.Context, req apiparams.RemoveGroupRequest) error {
	const op = errors.Op("jujuapi.RemoveGroup")
	if r.user.ControllerAccess != "superuser" {
		return errors.E(op, errors.CodeUnauthorized, "unauthorized")
	}

	group, err := r.jimm.Database.GetGroup(ctx, req.Name)
	if err != nil {
		return errors.E(op, err)
	}
	//TODO(Kian): Also remove all tuples containing group with confirmation message in the CLI.
	if err := r.jimm.Database.RemoveGroup(ctx, group); err != nil {
		zapctx.Error(ctx, "failed to remove group", zaputil.Error(err))
		return errors.E(op, err)
	}
	return nil
}

// ListGroup lists relational access control groups within JIMMs DB.
func (r *controllerRoot) ListGroups(ctx context.Context) (apiparams.ListGroupResponse, error) {
	const op = errors.Op("jujuapi.ListGroups")
	if r.user.ControllerAccess != "superuser" {
		return apiparams.ListGroupResponse{}, errors.E(op, errors.CodeUnauthorized, "unauthorized")
	}

	var groups []apiparams.Group
	err := r.jimm.Database.ForEachGroup(ctx, func(ctl *dbmodel.GroupEntry) error {
		groups = append(groups, ctl.ToAPIGroupEntry())
		return nil
	})
	if err != nil {
		return apiparams.ListGroupResponse{}, errors.E(op, err)
	}

	return apiparams.ListGroupResponse{Groups: groups}, nil
}

// AddRelation creates a tuple between two objects [if applicable]
// within OpenFGA.
func (r *controllerRoot) AddRelation(ctx context.Context) error {
	return errors.E("Not implemented.")
}

// RemoveRelation removes a tuple between two objects [if applicable]
// within OpenFGA.
func (r *controllerRoot) RemoveRelation(ctx context.Context) error {
	return errors.E("Not implemented.")
}

// CheckRelation performs an authorisation check for a particular group/user tuple
// against another tuple (This may be many tuples however, also known as a contextual tuple set.) within OpenFGA.
// This corresponds directly to /stores/{store_id}/check.
func (r *controllerRoot) CheckRelation(ctx context.Context) error {
	return errors.E("Not implemented.")
}

// ListRelations TODO(ale8k): Confirm validity / need for this when using /expand or [EXPERIMENTAL] /list-objects
//
// See: https://openfga.dev/api/service#/Relationship%20Queries/Expand
func (r *controllerRoot) ListRelations(ctx context.Context) error {
	return errors.E("Not implemented.")
}

// GetAuthorisationModel retrieves a GET for an authorisation model in the JIMM store
// by name.
//
// TODO(ale8k): Confirm web team can/is happy to display this.
// TODO(ale8k): Should this be paginated? Probably not?
func (r *controllerRoot) GetAuthorisationModel(ctx context.Context) error {
	return errors.E("Not implemented.")
}
