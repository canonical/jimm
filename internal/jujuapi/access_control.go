// Copyright 2023 canonical.

package jujuapi

import (
	"context"
	"regexp"
	"strconv"
	"time"

	"github.com/juju/zaputil"
	"github.com/juju/zaputil/zapctx"
	"go.uber.org/zap"

	"github.com/canonical/jimm/v3/internal/common/pagination"
	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/openfga"
	apiparams "github.com/canonical/jimm/v3/pkg/api/params"
	jimmnames "github.com/canonical/jimm/v3/pkg/names"
)

// access_control contains the primary RPC commands for handling ReBAC within JIMM via the JIMM facade itself.

var (
	// Matches juju uris, jimm user/group tags and UUIDs
	// Performs a single match and breaks the juju URI into 10 groups, each successive group is XORD to ensure we can run
	// this just once.
	// The groups are as so:
	// [0] - Entire match
	// [1] - tag
	// [2] - A single "-", ignored
	// [3] - Controller name OR user name OR group name
	// [4] - A single ":", ignored
	// [5] - Controller user / model owner
	// [6] - A single "/", ignored
	// [7] - Model name
	// [8] - A single ".", ignored
	// [9] - Application offer name
	// [10] - Relation specifier (i.e., #member)
	// A complete matcher example would look like so with square-brackets denoting groups and paranthsis denoting index:
	// (1)[controller](2)[-](3)[controller-1](4)[:](5)[alice@canonical.com-place](6)[/](7)[model-1](8)[.](9)[offer-1](10)[#relation-specifier]"
	// In the case of something like: user-alice@wonderland or group-alices-wonderland#member, it would look like so:
	// (1)[user](2)[-](3)[alices@wonderland]
	// (1)[group](2)[-](3)[alices-wonderland](10)[#member]
	// So if a group, user, UUID, controller name comes in, it will always be index 3 for them
	// and if a relation specifier is present, it will always be index 10
	jujuURIMatcher = regexp.MustCompile(`([a-zA-Z0-9]*)(\-|\z)([a-zA-Z0-9-@.]*)(\:|)([a-zA-Z0-9-@]*)(\/|)([a-zA-Z0-9-]*)(\.|)([a-zA-Z0-9-]*)([a-zA-Z#]*|\z)\z`)
)

const (
	jimmControllerName = "jimm"
)

type GroupService interface {
	AddGroup(ctx context.Context, user *openfga.User, name string) (*dbmodel.GroupEntry, error)
	GetGroupByID(ctx context.Context, user *openfga.User, uuid string) (dbmodel.GroupEntry, error)
	ListGroups(ctx context.Context, user *openfga.User, filter pagination.LimitOffsetPagination) ([]dbmodel.GroupEntry, error)
	RenameGroup(ctx context.Context, user *openfga.User, oldName, newName string) error
	RemoveGroup(ctx context.Context, user *openfga.User, name string) error
}

// AddGroup creates a group within JIMMs DB for reference by OpenFGA.
func (r *controllerRoot) AddGroup(ctx context.Context, req apiparams.AddGroupRequest) (apiparams.AddGroupResponse, error) {
	const op = errors.Op("jujuapi.AddGroup")
	resp := apiparams.AddGroupResponse{}

	if !jimmnames.IsValidGroupName(req.Name) {
		return resp, errors.E(op, errors.CodeBadRequest, "invalid group name")
	}

	groupEntry, err := r.jimm.AddGroup(ctx, r.user, req.Name)
	if err != nil {
		zapctx.Error(ctx, "failed to add group", zaputil.Error(err))
		return resp, errors.E(op, err)
	}
	resp = apiparams.AddGroupResponse{Group: apiparams.Group{
		Name:      groupEntry.Name,
		UUID:      groupEntry.UUID,
		CreatedAt: groupEntry.CreatedAt.Format(time.RFC3339),
		UpdatedAt: groupEntry.UpdatedAt.Format(time.RFC3339),
	}}

	return resp, nil
}

// RenameGroup renames a group within JIMMs DB for reference by OpenFGA.
func (r *controllerRoot) RenameGroup(ctx context.Context, req apiparams.RenameGroupRequest) error {
	const op = errors.Op("jujuapi.RenameGroup")

	if !jimmnames.IsValidGroupName(req.NewName) {
		return errors.E(op, errors.CodeBadRequest, "invalid group name")
	}

	if err := r.jimm.RenameGroup(ctx, r.user, req.Name, req.NewName); err != nil {
		zapctx.Error(ctx, "failed to rename group", zaputil.Error(err))
		return errors.E(op, err)
	}
	return nil
}

// RemoveGroup removes a group within JIMMs DB for reference by OpenFGA.
func (r *controllerRoot) RemoveGroup(ctx context.Context, req apiparams.RemoveGroupRequest) error {
	const op = errors.Op("jujuapi.RemoveGroup")

	if err := r.jimm.RemoveGroup(ctx, r.user, req.Name); err != nil {
		zapctx.Error(ctx, "failed to remove group", zaputil.Error(err))
		return errors.E(op, err)
	}
	return nil
}

// ListGroup lists relational access control groups within JIMMs DB.
func (r *controllerRoot) ListGroups(ctx context.Context, req apiparams.ListGroupsRequest) (apiparams.ListGroupResponse, error) {
	const op = errors.Op("jujuapi.ListGroups")

	filter := pagination.NewOffsetFilter(req.Limit, req.Offset)
	groups, err := r.jimm.ListGroups(ctx, r.user, filter)
	if err != nil {
		return apiparams.ListGroupResponse{}, errors.E(op, err)
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
	const op = errors.Op("jujuapi.AddRelation")

	if err := r.jimm.AddRelation(ctx, r.user, req.Tuples); err != nil {
		zapctx.Error(ctx, "failed to add relation", zaputil.Error(err))
		return errors.E(op, err)
	}
	return nil
}

// RemoveRelation removes a tuple between two objects [if applicable]
// within OpenFGA.
func (r *controllerRoot) RemoveRelation(ctx context.Context, req apiparams.RemoveRelationRequest) error {
	const op = errors.Op("jujuapi.RemoveRelation")

	err := r.jimm.RemoveRelation(ctx, r.user, req.Tuples)
	if err != nil {
		zapctx.Error(ctx, "failed to delete tuple(s)", zap.NamedError("remove-relation-error", err))
		return errors.E(op, err)
	}
	return nil
}

// CheckRelation performs an authorisation check for a particular group/user tuple
// against another tuple within OpenFGA.
// This corresponds directly to /stores/{store_id}/check.
func (r *controllerRoot) CheckRelation(ctx context.Context, req apiparams.CheckRelationRequest) (apiparams.CheckRelationResponse, error) {
	const op = errors.Op("jujuapi.CheckRelation")
	checkResp := apiparams.CheckRelationResponse{Allowed: false}

	allowed, err := r.jimm.CheckRelation(ctx, r.user, req.Tuple, false)
	if err != nil {
		zapctx.Error(ctx, "failed to check relation", zap.NamedError("check-relation-error", err))
		return checkResp, errors.E(op, err)
	}
	checkResp.Allowed = allowed
	zapctx.Debug(ctx, "check request", zap.String("allowed", strconv.FormatBool(allowed)))
	return checkResp, nil
}

// ListRelationshipTuples returns a list of tuples matching the specified filter.
func (r *controllerRoot) ListRelationshipTuples(ctx context.Context, req apiparams.ListRelationshipTuplesRequest) (apiparams.ListRelationshipTuplesResponse, error) {
	const op = errors.Op("jujuapi.ListRelationshipTuples")

	responseTuples, ct, err := r.jimm.ListRelationshipTuples(ctx, r.user, req.Tuple, req.PageSize, req.ContinuationToken)
	if err != nil {
		return apiparams.ListRelationshipTuplesResponse{}, errors.E(op, err)
	}
	errors := []string{}
	tuples := make([]apiparams.RelationshipTuple, len(responseTuples))
	for i, t := range responseTuples {
		object, err := r.jimm.ToJAASTag(ctx, t.Object)
		if err != nil {
			object = t.Object.String()
			errors = append(errors, "failed to parse object: "+err.Error())
		}
		target, err := r.jimm.ToJAASTag(ctx, t.Target)
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
