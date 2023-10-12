// Copyright 2023 canonical.

package jujuapi

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/names/v4"
	"github.com/juju/zaputil"
	"github.com/juju/zaputil/zapctx"
	"go.uber.org/zap"

	apiparams "github.com/canonical/jimm/api/params"
	"github.com/canonical/jimm/internal/dbmodel"
	"github.com/canonical/jimm/internal/errors"
	"github.com/canonical/jimm/internal/jimm"
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
	// (1)[controller](2)[-](3)[controller-1](4)[:](5)[alice@external-place](6)[/](7)[model-1](8)[.](9)[offer-1](10)[#relation-specifier]"
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

	isAdmin, err := openfga.IsAdministrator(ctx, r.user, r.jimm.ResourceTag())
	if err != nil {
		zapctx.Error(ctx, "openfga check failed", zap.Error(err))
		return errors.E(op, err)
	}
	if !isAdmin {
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

	if !jimmnames.IsValidGroupName(req.NewName) {
		return errors.E(op, errors.CodeBadRequest, "invalid group name")
	}

	isAdmin, err := openfga.IsAdministrator(ctx, r.user, r.jimm.ResourceTag())
	if err != nil {

		zapctx.Error(ctx, "openfga check failed", zap.Error(err))
		return errors.E(op, err)
	}
	if !isAdmin {
		return errors.E(op, errors.CodeUnauthorized, "unauthorized")
	}

	group := &dbmodel.GroupEntry{
		Name: req.Name,
	}
	err = r.jimm.Database.GetGroup(ctx, group)
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

	isAdmin, err := openfga.IsAdministrator(ctx, r.user, r.jimm.ResourceTag())
	if err != nil {
		zapctx.Error(ctx, "openfga check failed", zap.Error(err))
		return errors.E(op, err)
	}
	if !isAdmin {
		return errors.E(op, errors.CodeUnauthorized, "unauthorized")
	}

	group := &dbmodel.GroupEntry{
		Name: req.Name,
	}
	err = r.jimm.Database.GetGroup(ctx, group)
	if err != nil {
		return errors.E(op, err)
	}
	err = r.jimm.OpenFGAClient.RemoveGroup(ctx, group.ResourceTag())
	if err != nil {
		return errors.E(op, err)
	}

	if err := r.jimm.Database.RemoveGroup(ctx, group); err != nil {
		zapctx.Error(ctx, "failed to remove group", zaputil.Error(err))
		return errors.E(op, err)
	}
	return nil
}

// ListGroup lists relational access control groups within JIMMs DB.
func (r *controllerRoot) ListGroups(ctx context.Context) (apiparams.ListGroupResponse, error) {
	const op = errors.Op("jujuapi.ListGroups")

	isAdmin, err := openfga.IsAdministrator(ctx, r.user, r.jimm.ResourceTag())
	if err != nil {
		zapctx.Error(ctx, "openfga check failed", zap.Error(err))
		return apiparams.ListGroupResponse{}, errors.E(op, err)
	}
	if !isAdmin {
		return apiparams.ListGroupResponse{}, errors.E(op, errors.CodeUnauthorized, "unauthorized")
	}

	var groups []apiparams.Group
	err = r.jimm.Database.ForEachGroup(ctx, func(ctl *dbmodel.GroupEntry) error {
		groups = append(groups, ctl.ToAPIGroupEntry())
		return nil
	})
	if err != nil {
		return apiparams.ListGroupResponse{}, errors.E(op, err)
	}

	return apiparams.ListGroupResponse{Groups: groups}, nil
}

// resolveTag resolves JIMM tag [of any kind available] (i.e., controller-mycontroller:alex@external/mymodel.myoffer)
// into a juju string tag (i.e., controller-<controller uuid>).
//
// If the JIMM tag is aleady of juju string tag form, the transformation is left alone.
//
// In both cases though, the resource the tag pertains to is validated to exist within the database.
func resolveTag(j *jimm.JIMM, tag string) (*ofganames.Tag, error) {
	ctx := context.Background()
	matches := jujuURIMatcher.FindStringSubmatch(tag)
	resourceUUID := ""
	trailer := ""
	// We first attempt to see if group3 is a uuid
	if _, err := uuid.Parse(matches[3]); err == nil {
		// We know it's a UUID
		resourceUUID = matches[3]
	} else {
		// We presume it's a user or a group
		trailer = matches[3]
	}

	// Matchers along the way to determine segments of the string, they'll be empty
	// if the match has failed
	controllerName := matches[3]
	userName := matches[5]
	modelName := matches[7]
	offerName := matches[9]
	relationString := strings.TrimLeft(matches[10], "#")
	relation, err := ofganames.ParseRelation(relationString)
	if err != nil {
		return nil, errors.E("failed to parse relation", errors.CodeBadRequest)
	}

	switch matches[1] {
	case names.UserTagKind:
		zapctx.Debug(
			ctx,
			"Resolving JIMM tags to Juju tags for tag kind: user",
			zap.String("user-name", trailer),
		)
		return ofganames.ConvertTagWithRelation(names.NewUserTag(trailer), relation), nil

	case jimmnames.GroupTagKind:
		zapctx.Debug(
			ctx,
			"Resolving JIMM tags to Juju tags for tag kind: group",
			zap.String("group-name", trailer),
		)
		entry := &dbmodel.GroupEntry{
			Name: trailer,
		}
		err := j.Database.GetGroup(ctx, entry)
		if err != nil {
			return nil, errors.E("group not found")
		}
		return ofganames.ConvertTagWithRelation(jimmnames.NewGroupTag(strconv.FormatUint(uint64(entry.ID), 10)), relation), nil

	case names.ControllerTagKind:
		zapctx.Debug(
			ctx,
			"Resolving JIMM tags to Juju tags for tag kind: controller",
		)
		controller := dbmodel.Controller{}

		if resourceUUID != "" {
			controller.UUID = resourceUUID
		} else if controllerName != "" {
			if controllerName == jimmControllerName {
				return ofganames.ConvertTagWithRelation(names.NewControllerTag(j.UUID), relation), nil
			}
			controller.Name = controllerName
		}

		// NOTE (alesstimec) Do we need to special-case the
		// controller-jimm case - jimm controller does not exist
		// in the database, but has a clearly defined UUID?

		err := j.Database.GetController(ctx, &controller)
		if err != nil {
			return nil, errors.E("controller not found")
		}
		return ofganames.ConvertTagWithRelation(names.NewControllerTag(controller.UUID), relation), nil

	case names.ModelTagKind:
		zapctx.Debug(
			ctx,
			"Resolving JIMM tags to Juju tags for tag kind: model",
		)
		model := dbmodel.Model{}

		if resourceUUID != "" {
			model.UUID = sql.NullString{String: resourceUUID, Valid: true}
		} else if controllerName != "" && userName != "" && modelName != "" {
			controller := dbmodel.Controller{Name: controllerName}
			err := j.Database.GetController(ctx, &controller)
			if err != nil {
				return nil, errors.E("controller not found")
			}
			model.ControllerID = controller.ID
			model.OwnerUsername = userName
			model.Name = modelName
		}

		err := j.Database.GetModel(ctx, &model)
		if err != nil {
			return nil, errors.E("model not found")
		}

		return ofganames.ConvertTagWithRelation(names.NewModelTag(model.UUID.String), relation), nil

	case names.ApplicationOfferTagKind:
		zapctx.Debug(
			ctx,
			"Resolving JIMM tags to Juju tags for tag kind: applicationoffer",
		)
		offer := dbmodel.ApplicationOffer{}

		if resourceUUID != "" {
			offer.UUID = resourceUUID
		} else if controllerName != "" && userName != "" && modelName != "" && offerName != "" {
			offerURL, err := crossmodel.ParseOfferURL(fmt.Sprintf("%s:%s/%s.%s", controllerName, userName, modelName, offerName))
			if err != nil {
				zapctx.Debug(ctx, "failed to parse application offer url", zap.String("url", fmt.Sprintf("%s:%s/%s.%s", controllerName, userName, modelName, offerName)), zaputil.Error(err))
				return nil, errors.E("failed to parse offer url", err)
			}
			offer.URL = offerURL.String()
		}

		err := j.Database.GetApplicationOffer(ctx, &offer)
		if err != nil {
			return nil, errors.E("application offer not found")
		}

		return ofganames.ConvertTagWithRelation(names.NewApplicationOfferTag(offer.UUID), relation), nil
	}
	return nil, errors.E("failed to map tag " + matches[1])
}

// parseTag attempts to parse the provided key into a tag whilst additionally
// ensuring the resource exists for said tag.
//
// This key may be in the form of either a JIMM tag string or Juju tag string.
func parseTag(ctx context.Context, j *jimm.JIMM, key string) (*ofganames.Tag, error) {
	op := errors.Op("jujuapi.parseTag")
	tupleKeySplit := strings.SplitN(key, "-", 2)
	if len(tupleKeySplit) < 2 {
		return nil, errors.E(op, errors.CodeFailedToParseTupleKey, "tag does not have tuple key delimiter")
	}
	tagString := key
	tag, err := resolveTag(j, tagString)
	if err != nil {
		zapctx.Debug(ctx, "failed to resolve tuple object", zap.Error(err))
		return nil, errors.E(op, errors.CodeFailedToResolveTupleResource, err)
	}
	zapctx.Debug(ctx, "resolved JIMM tag", zap.String("tag", tag.String()))

	return tag, nil
}

// AddRelation creates a tuple between two objects [if applicable]
// within OpenFGA.
func (r *controllerRoot) AddRelation(ctx context.Context, req apiparams.AddRelationRequest) error {
	const op = errors.Op("jujuapi.AddRelation")

	isAdmin, err := openfga.IsAdministrator(ctx, r.user, r.jimm.ResourceTag())
	if err != nil {
		zapctx.Error(ctx, "openfga check failed", zap.Error(err))
		return errors.E(op, err)
	}
	if !isAdmin {
		return errors.E(op, errors.CodeUnauthorized, "unauthorized")
	}
	keys, err := r.parseTuples(ctx, req.Tuples)
	if err != nil {
		return errors.E(err)
	}
	err = r.jimm.OpenFGAClient.AddRelation(ctx, keys...)
	if err != nil {
		zapctx.Error(ctx, "failed to add tuple(s)", zap.NamedError("add-relation-error", err))
		return errors.E(op, errors.CodeOpenFGARequestFailed, err)
	}
	return nil
}

// RemoveRelation removes a tuple between two objects [if applicable]
// within OpenFGA.
func (r *controllerRoot) RemoveRelation(ctx context.Context, req apiparams.RemoveRelationRequest) error {
	const op = errors.Op("jujuapi.RemoveRelation")

	isAdmin, err := openfga.IsAdministrator(ctx, r.user, r.jimm.ResourceTag())
	if err != nil {
		zapctx.Error(ctx, "openfga check failed", zap.Error(err))
		return errors.E(op, err)
	}
	if !isAdmin {
		return errors.E(op, errors.CodeUnauthorized, "unauthorized")
	}
	keys, err := r.parseTuples(ctx, req.Tuples)
	if err != nil {
		return errors.E(op, err)
	}
	err = r.jimm.OpenFGAClient.RemoveRelation(ctx, keys...)
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

	isAdmin, err := openfga.IsAdministrator(ctx, r.user, r.jimm.ResourceTag())
	if err != nil {
		zapctx.Error(ctx, "openfga check failed", zap.Error(err))
		return checkResp, errors.E(op, err)
	}
	if !isAdmin {
		return checkResp, errors.E(op, errors.CodeUnauthorized, "unauthorized")
	}

	parsedTuple, err := r.parseTuple(ctx, req.Tuple)
	if err != nil {
		return checkResp, errors.E(op, errors.CodeFailedToParseTupleKey, err)
	}

	allowed, err := r.jimm.OpenFGAClient.CheckRelation(ctx, *parsedTuple, false)
	if err != nil {
		zapctx.Error(ctx, "failed to check relation", zap.NamedError("check-relation-error", err))
		return checkResp, errors.E(op, errors.CodeOpenFGARequestFailed, err)
	}
	if allowed {
		checkResp.Allowed = allowed
	}
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
		targetTag, err := parseTag(ctx, r.jimm, tuple.TargetObject)
		if err != nil {
			return nil, parseTagError("failed to parse tuple target object key", tuple.TargetObject, err)
		}
		t.Target = targetTag
	}
	if tuple.Object != "" {
		objectTag, err := parseTag(ctx, r.jimm, tuple.Object)
		if err != nil {
			return nil, parseTagError("failed to parse tuple object key", tuple.Object, err)
		}
		t.Object = objectTag
	}

	return &t, nil
}

func (r *controllerRoot) toJAASTag(ctx context.Context, tag *ofganames.Tag) (string, error) {
	switch tag.Kind {
	case names.UserTagKind:
		return names.UserTagKind + "-" + tag.ID, nil
	case names.ControllerTagKind:
		if tag.ID == r.jimm.ResourceTag().Id() {
			return "controller-jimm", nil
		}
		controller := dbmodel.Controller{
			UUID: tag.ID,
		}
		err := r.jimm.Database.GetController(ctx, &controller)
		if err != nil {
			return "", errors.E(err, fmt.Sprintf("failed to fetch controller information: %s", controller.UUID))
		}
		controllerString := names.ControllerTagKind + "-" + controller.Name
		if tag.Relation.String() != "" {
			controllerString = controllerString + "#" + tag.Relation.String()
		}
		return controllerString, nil
	case names.ModelTagKind:
		model := dbmodel.Model{
			UUID: sql.NullString{
				String: tag.ID,
				Valid:  true,
			},
		}
		err := r.jimm.Database.GetModel(ctx, &model)
		if err != nil {
			return "", errors.E(err, "failed to fetch model information")
		}
		modelString := names.ModelTagKind + "-" + model.Controller.Name + ":" + model.OwnerUsername + "/" + model.Name
		if tag.Relation.String() != "" {
			modelString = modelString + "#" + tag.Relation.String()
		}
		return modelString, nil
	case names.ApplicationOfferTagKind:
		ao := dbmodel.ApplicationOffer{
			UUID: tag.ID,
		}
		err := r.jimm.Database.GetApplicationOffer(ctx, &ao)
		if err != nil {
			return "", errors.E(err, "failed to fetch application offer information")
		}
		aoString := names.ApplicationOfferTagKind + "-" + ao.Model.Controller.Name + ":" + ao.Model.OwnerUsername + "/" + ao.Model.Name + "." + ao.Name
		if tag.Relation.String() != "" {
			aoString = aoString + "#" + tag.Relation.String()
		}
		return aoString, nil
	case jimmnames.GroupTagKind:
		id, err := strconv.ParseUint(tag.ID, 10, 32)
		if err != nil {
			return "", errors.E(err, fmt.Sprintf("failed to parse group id: %v", tag.ID))
		}
		group := dbmodel.GroupEntry{
			ID: uint(id),
		}
		err = r.jimm.Database.GetGroup(ctx, &group)
		if err != nil {
			return "", errors.E(err, "failed to fetch group information")
		}
		groupString := jimmnames.GroupTagKind + "-" + group.Name
		if tag.Relation.String() != "" {
			groupString = groupString + "#" + tag.Relation.String()
		}
		return groupString, nil
	case names.CloudTagKind:
		cloud := dbmodel.Cloud{
			Name: tag.ID,
		}
		err := r.jimm.Database.GetCloud(ctx, &cloud)
		if err != nil {
			return "", errors.E(err, "failed to fetch group information")
		}
		cloudString := names.CloudTagKind + "-" + cloud.Name
		if tag.Relation.String() != "" {
			cloudString = cloudString + "#" + tag.Relation.String()
		}
		return cloudString, nil
	default:
		return "", errors.E(fmt.Sprintf("unexpected tag kind: %v", tag.Kind))
	}
}

// ListRelationshipTuples returns a list of tuples matching the specified filter.
func (r *controllerRoot) ListRelationshipTuples(ctx context.Context, req apiparams.ListRelationshipTuplesRequest) (apiparams.ListRelationshipTuplesResponse, error) {
	const op = errors.Op("jujuapi.ListRelationshipTuples")
	var returnValue apiparams.ListRelationshipTuplesResponse

	isAdmin, err := openfga.IsAdministrator(ctx, r.user, r.jimm.ResourceTag())
	if err != nil {
		zapctx.Error(ctx, "openfga check failed", zap.Error(err))
		return returnValue, errors.E(op, err)
	}
	if !isAdmin {
		return returnValue, errors.E(op, errors.CodeUnauthorized, "unauthorized")
	}

	key := &openfga.Tuple{}
	if req.Tuple.TargetObject != "" {
		key, err = r.parseTuple(ctx, req.Tuple)
		if err != nil {
			return returnValue, errors.E(op, err)
		}
	}
	responseTuples, ct, err := r.jimm.OpenFGAClient.ReadRelatedObjects(ctx, *key, req.PageSize, req.ContinuationToken)
	if err != nil {
		return returnValue, errors.E(op, err)
	}
	tuples := make([]apiparams.RelationshipTuple, len(responseTuples))
	for i, t := range responseTuples {
		object, err := r.toJAASTag(ctx, t.Object)
		if err != nil {
			return returnValue, errors.E(op, err)
		}
		target, err := r.toJAASTag(ctx, t.Target)
		if err != nil {
			return returnValue, errors.E(op, err)
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
	}, nil
}
