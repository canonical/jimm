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

	apiparams "github.com/canonical/jimm/api/params"
	"github.com/canonical/jimm/internal/common/pagination"
	"github.com/canonical/jimm/internal/errors"
	"github.com/canonical/jimm/internal/openfga"
	ofganames "github.com/canonical/jimm/internal/openfga/names"
	jimmnames "github.com/canonical/jimm/pkg/names"
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

// AddGroup creates a group within JIMMs DB for reference by OpenFGA.
func (r *controllerRoot) AddGroup(ctx context.Context, req apiparams.AddGroupRequest) error {
	const op = errors.Op("jujuapi.AddGroup")

	if !jimmnames.IsValidGroupName(req.Name) {
		return errors.E(op, errors.CodeBadRequest, "invalid group name")
	}
	if err := r.jimm.AddGroup(ctx, r.user, req.Name); err != nil {
		zapctx.Error(ctx, "failed to add group", zaputil.Error(err))
		return errors.E(op, err)
	}
	return nil
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
			Name:      g.Name,
			UUID:      g.UUID,
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
	keys, err := r.parseTuples(ctx, req.Tuples)
	if err != nil {
		return errors.E(err)
	}
	if err := r.jimm.AddRelation(ctx, r.user, keys); err != nil {
		zapctx.Error(ctx, "failed to add relation", zaputil.Error(err))
		return errors.E(op, err)
	}
	return nil
}

// RemoveRelation removes a tuple between two objects [if applicable]
// within OpenFGA.
func (r *controllerRoot) RemoveRelation(ctx context.Context, req apiparams.RemoveRelationRequest) error {
	const op = errors.Op("jujuapi.RemoveRelation")

	keys, err := r.parseTuples(ctx, req.Tuples)
	if err != nil {
		return errors.E(op, err)
	}
	err = r.jimm.RemoveRelation(ctx, r.user, keys)
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

	parsedTuple, err := r.parseTuple(ctx, req.Tuple)
	if err != nil {
		return checkResp, errors.E(op, errors.CodeFailedToParseTupleKey, err)
	}

	allowed, err := r.jimm.CheckRelation(ctx, r.user, *parsedTuple, false)
	if err != nil {
		zapctx.Error(ctx, "failed to check relation", zap.NamedError("check-relation-error", err))
		return checkResp, errors.E(op, errors.CodeOpenFGARequestFailed, err)
	}
	checkResp.Allowed = allowed
	zapctx.Debug(ctx, "check request", zap.String("allowed", strconv.FormatBool(allowed)))
	return checkResp, nil
}

// parseTuples translate the api request struct containing tuples to a slice of openfga tuple keys.
// This method utilises the parseTuple method which does all the heavy lifting.
func (r *controllerRoot) parseTuples(ctx context.Context, tuples []apiparams.RelationshipTuple) ([]openfga.Tuple, error) {
	keys := make([]openfga.Tuple, 0, len(tuples))
	for _, tuple := range tuples {
		key, err := r.parseTuple(ctx, tuple)
		if err != nil {
			return nil, errors.E(err)
		}
		keys = append(keys, *key)
	}
	return keys, nil
}

// parseTuple takes the initial tuple from a relational request and ensures that
// whatever format, be it JAAS or Juju tag, is resolved to the correct identifier
// to be persisted within OpenFGA.
func (r *controllerRoot) parseTuple(ctx context.Context, tuple apiparams.RelationshipTuple) (*openfga.Tuple, error) {
	const op = errors.Op("jujuapi.parseTuple")

	relation, err := ofganames.ParseRelation(tuple.Relation)
	if err != nil {
		return nil, errors.E(op, err, errors.CodeBadRequest)
	}
	t := openfga.Tuple{
		Relation: relation,
	}

	// Wraps the general error that will be sent for both
	// the object and target object, but changing the message and key
	// to be specific to the erroneous offender.
	parseTagError := func(msg string, key string, err error) error {
		zapctx.Debug(ctx, msg, zap.String("key", key), zap.Error(err))
		return errors.E(op, errors.CodeFailedToParseTupleKey, err, msg+" "+key)
	}

	if tuple.TargetObject == "" {
		return nil, errors.E(op, errors.CodeBadRequest, "target object not specified")
	}
	if tuple.TargetObject != "" {
		targetTag, err := r.jimm.ParseTag(ctx, tuple.TargetObject)
		if err != nil {
			return nil, parseTagError("failed to parse tuple target object key", tuple.TargetObject, err)
		}
		t.Target = targetTag
	}
	if tuple.Object != "" {
		objectTag, err := r.jimm.ParseTag(ctx, tuple.Object)
		if err != nil {
			return nil, parseTagError("failed to parse tuple object key", tuple.Object, err)
		}
		t.Object = objectTag
	}

	return &t, nil
}

// ListRelationshipTuples returns a list of tuples matching the specified filter.
func (r *controllerRoot) ListRelationshipTuples(ctx context.Context, req apiparams.ListRelationshipTuplesRequest) (apiparams.ListRelationshipTuplesResponse, error) {
	const op = errors.Op("jujuapi.ListRelationshipTuples")

	key := &openfga.Tuple{}
	var err error
	if req.Tuple.TargetObject != "" {
		key, err = r.parseTuple(ctx, req.Tuple)
		if err != nil {
			return apiparams.ListRelationshipTuplesResponse{}, errors.E(op, err)
		}
	}
	responseTuples, ct, err := r.jimm.ListRelationshipTuples(ctx, r.user, *key, req.PageSize, req.ContinuationToken)
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
