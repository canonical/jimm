// Copyright 2025 Canonical.

package jujuapi

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/juju/zaputil/zapctx"
	"go.uber.org/zap"

	"github.com/canonical/jimm/v3/internal/common/pagination"
	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
	apiparams "github.com/canonical/jimm/v3/pkg/api/params"
	jimmnames "github.com/canonical/jimm/v3/pkg/names"
)

// access_control contains the primary RPC commands for handling ReBAC within JIMM via the JIMM facade itself.

const (
	jimmControllerName = "jimm"
)

// AddGroup creates a group within JIMMs DB for reference by OpenFGA.
func (r *controllerRoot) AddGroup(ctx context.Context, req apiparams.AddGroupRequest) (apiparams.AddGroupResponse, error) {

	resp := apiparams.AddGroupResponse{}

	if !jimmnames.IsValidGroupName(req.Name) {
		return resp, errors.E(errors.CodeBadRequest, "invalid group name")
	}

	groupEntry, err := r.jimm.GroupManager().AddGroup(ctx, r.user, req.Name)
	if err != nil {
		return resp, errors.E(fmt.Errorf("failed to add group: %w", err))
	}
	resp = apiparams.AddGroupResponse{Group: apiparams.Group{
		Name:      groupEntry.Name,
		UUID:      groupEntry.UUID,
		CreatedAt: groupEntry.CreatedAt.Format(time.RFC3339),
		UpdatedAt: groupEntry.UpdatedAt.Format(time.RFC3339),
	}}

	return resp, nil
}

// GetGroup returns group information based on a UUID or name.
func (r *controllerRoot) GetGroup(ctx context.Context, req apiparams.GetGroupRequest) (apiparams.Group, error) {

	var groupEntry *dbmodel.GroupEntry
	var err error
	switch {
	case req.UUID != "" && req.Name != "":
		return apiparams.Group{}, errors.E(errors.CodeBadRequest, "only one of UUID or Name should be provided")
	case req.UUID != "":
		groupEntry, err = r.jimm.GroupManager().GetGroupByUUID(ctx, r.user, req.UUID)
	case req.Name != "":
		groupEntry, err = r.jimm.GroupManager().GetGroupByName(ctx, r.user, req.Name)
	default:
		return apiparams.Group{}, errors.E(errors.CodeBadRequest, "no UUID or Name provided")
	}
	if err != nil {
		return apiparams.Group{}, errors.E(fmt.Errorf("failed to get group: %w", err))
	}

	return apiparams.Group{
		UUID:      groupEntry.UUID,
		Name:      groupEntry.Name,
		CreatedAt: groupEntry.CreatedAt.Format(time.RFC3339),
		UpdatedAt: groupEntry.UpdatedAt.Format(time.RFC3339),
	}, nil
}

// RenameGroup renames a group within JIMMs DB for reference by OpenFGA.
func (r *controllerRoot) RenameGroup(ctx context.Context, req apiparams.RenameGroupRequest) error {

	if !jimmnames.IsValidGroupName(req.NewName) {
		return errors.E(errors.CodeBadRequest, "invalid group name")
	}

	if err := r.jimm.GroupManager().RenameGroup(ctx, r.user, req.Name, req.NewName); err != nil {
		return errors.E(fmt.Errorf("failed to rename group: %w", err))
	}
	return nil
}

// RemoveGroup removes a group within JIMMs DB for reference by OpenFGA.
func (r *controllerRoot) RemoveGroup(ctx context.Context, req apiparams.RemoveGroupRequest) error {

	if err := r.jimm.GroupManager().RemoveGroup(ctx, r.user, req.Name); err != nil {
		return errors.E(fmt.Errorf("failed to remove group: %w", err))
	}
	return nil
}

// ListGroup lists relational access control groups within JIMMs DB.
func (r *controllerRoot) ListGroups(ctx context.Context, req apiparams.ListGroupsRequest) (apiparams.ListGroupResponse, error) {

	pagination := pagination.NewOffsetFilter(req.Limit, req.Offset)
	groups, err := r.jimm.GroupManager().ListGroups(ctx, r.user, pagination, "")
	if err != nil {
		return apiparams.ListGroupResponse{}, errors.E(fmt.Errorf("failed to list groups: %w", err))
	}
	groupsResponse := make([]apiparams.Group, len(groups))
	for i, g := range groups {
		groupsResponse[i] = apiparams.Group{
			UUID:      g.UUID,
			Name:      g.Name,
			CreatedAt: g.CreatedAt.Format(time.RFC3339),
			UpdatedAt: g.UpdatedAt.Format(time.RFC3339),
		}
	}

	return apiparams.ListGroupResponse{Groups: groupsResponse}, nil
}

// AddRelation creates a tuple between two objects [if applicable]
// within OpenFGA.
func (r *controllerRoot) AddRelation(ctx context.Context, req apiparams.AddRelationRequest) error {

	if err := r.jimm.PermissionManager().AddRelation(ctx, r.user, req.Tuples); err != nil {
		return errors.E(fmt.Errorf("failed to add relation: %w", err))
	}
	return nil
}

// RemoveRelation removes a tuple between two objects [if applicable]
// within OpenFGA.
func (r *controllerRoot) RemoveRelation(ctx context.Context, req apiparams.RemoveRelationRequest) error {

	err := r.jimm.PermissionManager().RemoveRelation(ctx, r.user, req.Tuples)
	if err != nil {
		return errors.E(fmt.Errorf("failed to remove relation: %w", err))
	}
	return nil
}

// CheckRelation performs an authorisation check for a particular group/user tuple
// against another tuple within OpenFGA.
// This corresponds directly to /stores/{store_id}/check.
func (r *controllerRoot) CheckRelation(ctx context.Context, req apiparams.CheckRelationRequest) (apiparams.CheckRelationResponse, error) {

	checkResp := apiparams.CheckRelationResponse{Allowed: false}

	allowed, err := r.jimm.PermissionManager().CheckRelation(ctx, r.user, req.Tuple, false)
	if err != nil {
		checkResp.Error = err.Error()
		return checkResp, errors.E(fmt.Errorf("failed to check relation: %w", err))
	}
	checkResp.Allowed = allowed
	zapctx.Debug(ctx, "check request", zap.String("allowed", strconv.FormatBool(allowed)))
	return checkResp, nil
}

// CheckRelations performs an authorisation check for a list of tuples.
// It returns a list of results, each with an Allowed boolean and an optional error message.
func (r *controllerRoot) CheckRelations(ctx context.Context, req apiparams.CheckRelationsRequest) (apiparams.CheckRelationsResponse, error) {

	checksResp := apiparams.CheckRelationsResponse{}

	results, err := r.jimm.PermissionManager().CheckRelations(ctx, r.user, req.Tuples)
	if err != nil {
		return checksResp, errors.E(fmt.Errorf("failed to check relations: %w", err))
	}
	for _, result := range results {
		resp := apiparams.CheckRelationResponse{
			Allowed: result.Allowed,
		}
		if result.Error != nil {
			resp.Error = result.Error.Error()
		}
		checksResp.Results = append(checksResp.Results, resp)
	}

	return checksResp, nil
}

// ListRelationshipTuples returns a list of tuples matching the specified filter.
func (r *controllerRoot) ListRelationshipTuples(ctx context.Context, req apiparams.ListRelationshipTuplesRequest) (apiparams.ListRelationshipTuplesResponse, error) {

	responseTuples, ct, err := r.jimm.PermissionManager().ListRelationshipTuples(ctx, r.user, req.Tuple, req.PageSize, req.ContinuationToken)
	if err != nil {
		return apiparams.ListRelationshipTuplesResponse{}, errors.E(fmt.Errorf("failed to list relations: %w", err))
	}
	errors := []string{}
	tuples := make([]apiparams.RelationshipTuple, len(responseTuples))
	for i, t := range responseTuples {
		object, err := r.jimm.PermissionManager().ToJAASTag(ctx, t.Object, req.ResolveUUIDs)
		if err != nil {
			object = t.Object.String()
			errors = append(errors, "failed to parse object: "+err.Error())
		}
		target, err := r.jimm.PermissionManager().ToJAASTag(ctx, t.Target, req.ResolveUUIDs)
		if err != nil {
			target = t.Target.String()
			errors = append(errors, "failed to parse target: "+err.Error())
		}
		tuples[i] = apiparams.RelationshipTuple{
			Object:       object,
			Relation:     string(t.Relation),
			TargetObject: target,
		}
	}
	return apiparams.ListRelationshipTuplesResponse{
		Tuples:            tuples,
		ContinuationToken: ct,
		Errors:            errors,
	}, nil
}
