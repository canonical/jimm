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
	openfga "github.com/openfga/go-sdk"
	"go.uber.org/zap"
	"gorm.io/gorm"

	apiparams "github.com/CanonicalLtd/jimm/api/params"
	"github.com/CanonicalLtd/jimm/internal/db"
	"github.com/CanonicalLtd/jimm/internal/dbmodel"
	"github.com/CanonicalLtd/jimm/internal/errors"
	ofga "github.com/CanonicalLtd/jimm/internal/openfga"
	jimmnames "github.com/CanonicalLtd/jimm/pkg/names"
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
	jujuURIMatcher = regexp.MustCompile(`([a-zA-Z0-9]*)(\-|\z)([a-zA-Z0-9-@]*)(\:|)([a-zA-Z0-9-@]*)(\/|)([a-zA-Z0-9-]*)(\.|)([a-zA-Z0-9-]*)([a-zA-Z#]*|\z)\z`)
)

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

	group := &dbmodel.GroupEntry{
		Name: req.Name,
	}
	err := r.jimm.Database.GetGroup(ctx, group)
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

	group := &dbmodel.GroupEntry{
		Name: req.Name,
	}
	err := r.jimm.Database.GetGroup(ctx, group)
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

// resolveTupleObject resolves JIMM tag [of any kind available] (i.e., controller-mycontroller:alex@external/mymodel.myoffer)
// into a juju string tag (i.e., controller-<controller uuid>).
//
// If the JIMM tag is aleady of juju string tag form, the transformation is left alone.
//
// In both cases though, the resource the tag pertains to is validated to exist within the database.
func resolveTupleObject(db db.Database, tag string) (string, string, error) {
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
	relationSpecifier := matches[10]

	switch matches[1] {
	case names.UserTagKind:
		zapctx.Debug(
			ctx,
			"Resolving JIMM tags to Juju tags for tag kind: user",
			zap.String("user-name", trailer),
		)
		return names.NewUserTag(trailer).String(), relationSpecifier, nil

	case jimmnames.GroupTagKind:
		zapctx.Debug(
			ctx,
			"Resolving JIMM tags to Juju tags for tag kind: group",
			zap.String("group-name", trailer),
		)
		entry := &dbmodel.GroupEntry{
			Name: trailer,
		}
		err := db.GetGroup(ctx, entry)
		if err != nil {
			return tag, relationSpecifier, errors.E("group not found")
		}
		return jimmnames.NewGroupTag(strconv.FormatUint(uint64(entry.ID), 10)).String(), relationSpecifier, nil

	case names.ControllerTagKind:
		zapctx.Debug(
			ctx,
			"Resolving JIMM tags to Juju tags for tag kind: controller",
		)
		controller := dbmodel.Controller{}

		if resourceUUID != "" {
			controller.UUID = resourceUUID
		} else if controllerName != "" {
			controller.Name = controllerName
		}

		err := db.GetController(ctx, &controller)
		if err != nil {
			return tag, relationSpecifier, errors.E("controller not found")
		}
		return names.NewControllerTag(controller.UUID).String(), relationSpecifier, nil

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
			err := db.GetController(ctx, &controller)
			if err != nil {
				return tag, relationSpecifier, errors.E("controller not found")
			}
			model.ControllerID = controller.ID
			model.OwnerUsername = userName
			model.Name = modelName
		}

		err := db.GetModel(ctx, &model)
		if err != nil {
			return tag, relationSpecifier, errors.E("model not found")
		}

		return names.NewModelTag(model.UUID.String).String(), relationSpecifier, nil

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
				return tag, relationSpecifier, errors.E("failed to parse offer url")
			}
			offer.URL = offerURL.String()
		}

		err := db.GetApplicationOffer(ctx, &offer)
		if err != nil {
			return tag, relationSpecifier, errors.E("application offer not found")
		}

		return jimmnames.NewApplicationOfferTag(offer.UUID).String(), relationSpecifier, nil
	}
	return tag, relationSpecifier, errors.E("failed to map tag")
}

// jujuTagFromTuple attempts to parse the provided objectId
// into a juju tag, and returns an error if this is not possible.
func jujuTagFromTuple(objectType string, objectId string) (names.Tag, error) {
	switch objectType {
	case names.UserTagKind:
		return names.ParseUserTag(objectId)
	case names.ModelTagKind:
		return names.ParseModelTag(objectId)
	case names.ControllerTagKind:
		return names.ParseControllerTag(objectId)
	case names.ApplicationOfferTagKind:
		return jimmnames.ParseApplicationOfferTag(objectId)
	case jimmnames.GroupTagKind:
		return jimmnames.ParseGroupTag(objectId)
	default:
		return nil, errors.E("could not determine tag type")
	}
}

// parseTag attempts to parse the provided key into a tag whilst additionally
// ensuring the resource exists for said tag.
//
// This key may be in the form of either a JIMM tag string or Juju tag string.
func parseTag(ctx context.Context, db db.Database, key string) (names.Tag, string, error) {
	tupleKeySplit := strings.SplitN(key, "-", 2)
	if len(tupleKeySplit) != 2 {
		return nil, "", errors.E("tag does not have tuple key delimiter")
	}
	kind := tupleKeySplit[0]
	tagString := key
	tagString, relationSpecifier, err := resolveTupleObject(db, tagString)
	if err != nil {
		zapctx.Debug(ctx, "failed to resolve tuple object", zap.Error(err))
		return nil, "", errors.E("failed to resolve tuple object: " + err.Error())
	}
	tag, err := jujuTagFromTuple(kind, tagString)
	return tag, relationSpecifier, err
}

// AddRelation creates a tuple between two objects [if applicable]
// within OpenFGA.
func (r *controllerRoot) AddRelation(ctx context.Context, req apiparams.AddRelationRequest) error {
	const op = errors.Op("jujuapi.AddRelation")
	if r.user.ControllerAccess != "superuser" {
		return errors.E(op, errors.CodeUnauthorized, "unauthorized")
	}
	keys, err := r.createTupleKeys(ctx, req.Tuples)
	if err != nil {
		return errors.E(err)
	}
	err = r.ofgaClient.AddRelations(ctx, keys...)
	if err != nil {
		zapctx.Error(ctx, "failed to add tuple(s)", zap.NamedError("add-relation-error", err))
		return errors.E(op, err)
	}
	return nil
}

// RemoveRelation removes a tuple between two objects [if applicable]
// within OpenFGA.
func (r *controllerRoot) RemoveRelation(ctx context.Context, req apiparams.RemoveRelationRequest) error {
	const op = errors.Op("jujuapi.RemoveRelation")
	keys, err := r.createTupleKeys(ctx, req.Tuples)
	if err != nil {
		return errors.E(op, err)
	}
	err = r.ofgaClient.RemoveRelation(ctx, keys...)
	if err != nil {
		zapctx.Error(ctx, "failed to delete tuple(s)", zap.NamedError("delete-relation-error", err))
		return errors.E(op, err)
	}
	return nil
}

// CheckRelation performs an authorisation check for a particular group/user tuple
// against another tuple (This may be many tuples however, also known as a contextual tuple set.) within OpenFGA.
// This corresponds directly to /stores/{store_id}/check.
func (r *controllerRoot) CheckRelation(ctx context.Context) error {
	return errors.E("Not implemented.")
}

// createTupleKeys translate the api request struct containing tuples to a slice of openfga tuple keys.
// This method utilises the parseTuple method which does all the heavy lifting.
func (r *controllerRoot) createTupleKeys(ctx context.Context, tuples []apiparams.RelationshipTuple) ([]openfga.TupleKey, error) {
	keys := make([]openfga.TupleKey, 0, len(tuples))
	for _, tuple := range tuples {
		key, err := r.parseTuple(ctx, tuple)
		if err != nil {
			errors.E(err)
		}
		keys = append(keys, *key)
	}
	return keys, nil
}

func (r *controllerRoot) parseTuple(ctx context.Context, tuple apiparams.RelationshipTuple) (*openfga.TupleKey, error) {
	const op = errors.Op("jujuapi.parseTuple")
	var objectString, targetString string
	if tuple.TargetObject == "" {
		return nil, errors.E(op, errors.CodeBadRequest, "target object not specified")
	}
	if tuple.TargetObject != "" {
		targetObject, targetObjectRelationSpecifier, err := parseTag(ctx, r.jimm.Database, tuple.TargetObject)
		if err != nil {
			zapctx.Debug(ctx, "failed to parse tuple object key", zap.String("key", tuple.TargetObject), zap.Error(err))
			return nil, errors.E(op, errors.CodeFailedToParseTupleKey, err, "failed to parse tuple object key: "+tuple.TargetObject)
		}
		targetString = targetObject.Kind() + ":" + targetObject.Id() + targetObjectRelationSpecifier
	}
	if tuple.Object != "" {
		objectTag, objectTagRelationSpecifier, err := parseTag(ctx, r.jimm.Database, tuple.Object)
		if err != nil {
			zapctx.Debug(ctx, "failed to parse tuple user key", zap.String("key", tuple.Object), zap.Error(err))
			return nil, errors.E(op, errors.CodeFailedToParseTupleKey, err, "failed to parse tuple user key: "+tuple.Object)
		}
		objectString = objectTag.Kind() + ":" + objectTag.Id() + objectTagRelationSpecifier
	}

	t := ofga.CreateTupleKey(
		objectString,
		tuple.Relation,
		targetString,
	)
	return &t, nil
}

func (r *controllerRoot) toJAASTag(ctx context.Context, tag string) (string, error) {
	tokens := strings.Split(tag, ":")
	if len(tokens) != 2 {
		return "", errors.E("unexpected tag format")
	}
	tokens2 := strings.Split(tokens[1], "#")
	if len(tokens2) == 0 || len(tokens2) > 2 {
		return "", errors.E("unexpected tag format")
	}
	switch tokens[0] {
	case names.UserTagKind:
		return names.UserTagKind + "-" + tokens[1], nil
	case names.ControllerTagKind:
		controller := dbmodel.Controller{
			UUID: tokens2[0],
		}
		err := r.jimm.Database.GetController(ctx, &controller)
		if err != nil {
			return "", errors.E(err, "failed to fetch controller information")
		}
		controllerString := names.ControllerTagKind + "-" + controller.Name
		if len(tokens2) == 2 {
			controllerString = controllerString + "#" + tokens2[1]
		}
		return controllerString, nil
	case names.ModelTagKind:
		model := dbmodel.Model{
			UUID: sql.NullString{
				String: tokens2[0],
				Valid:  true,
			},
		}
		err := r.jimm.Database.GetModel(ctx, &model)
		if err != nil {
			return "", errors.E(err, "failed to fetch model information")
		}
		modelString := names.ModelTagKind + "-" + model.Controller.Name + ":" + model.OwnerUsername + "/" + model.Name
		if len(tokens2) == 2 {
			modelString = modelString + "#" + tokens2[1]
		}
		return modelString, nil
	case names.ApplicationOfferTagKind:
		ao := dbmodel.ApplicationOffer{
			UUID: tokens2[0],
		}
		err := r.jimm.Database.GetApplicationOffer(ctx, &ao)
		if err != nil {
			return "", errors.E(err, "failed to fetch application offer information")
		}
		aoString := names.ApplicationOfferTagKind + "-" + ao.Model.Controller.Name + ":" + ao.Model.OwnerUsername + "/" + ao.Model.Name + "." + ao.Name
		if len(tokens2) == 2 {
			aoString = aoString + "#" + tokens2[1]
		}
		return aoString, nil
	case jimmnames.GroupTagKind:
		id, err := strconv.ParseUint(tokens2[0], 10, 32)
		if err != nil {
			return "", errors.E(err, "failed to parse group id")
		}
		group := dbmodel.GroupEntry{
			Model: gorm.Model{
				ID: uint(id),
			},
		}
		err = r.jimm.Database.GetGroup(ctx, &group)
		if err != nil {
			return "", errors.E(err, "failed to fetch group information")
		}
		groupString := jimmnames.GroupTagKind + "-" + group.Name
		if len(tokens2) == 2 {
			groupString = groupString + "#" + tokens2[1]
		}
		return groupString, nil
	default:
		return "", errors.E("unexpected tag kind: " + tokens[0])
	}
}

// ListRelationshipTuples returns a list of tuples matching the specified filter.
func (r *controllerRoot) ListRelationshipTuples(ctx context.Context, req apiparams.ListRelationshipTuplesRequest) (apiparams.ListRelationshipTuplesResponse, error) {
	const op = errors.Op("jujuapi.ListRelationshipTuples")
	var returnValue apiparams.ListRelationshipTuplesResponse

	if r.user.ControllerAccess != "superuser" {
		return returnValue, errors.E(op, errors.CodeUnauthorized, "unauthorized")
	}

	var key *openfga.TupleKey
	var err error
	if req.Tuple.TargetObject != "" {
		key, err = r.parseTuple(ctx, req.Tuple)
		if err != nil {
			if errors.ErrorCode(err) == errors.CodeFailedToParseTupleKey {
				return returnValue, errors.E(op, errors.CodeBadRequest, "failed to parse the tuple key")
			}
			return returnValue, errors.E(op, err)
		}
	}
	response, err := r.ofgaClient.ReadRelatedObjects(ctx, key, req.PageSize, req.ContinuationToken)
	if err != nil {
		return returnValue, errors.E(op, err)
	}
	tuples := make([]apiparams.RelationshipTuple, len(response.Keys))
	for i, t := range response.Keys {
		object, err := r.toJAASTag(ctx, t.GetUser())
		if err != nil {
			return returnValue, errors.E(op, err)
		}
		target, err := r.toJAASTag(ctx, t.GetObject())
		if err != nil {
			return returnValue, errors.E(op, err)
		}
		tuples[i] = apiparams.RelationshipTuple{
			Object:       object,
			Relation:     t.GetRelation(),
			TargetObject: target,
		}
	}
	return apiparams.ListRelationshipTuplesResponse{
		Tuples:            tuples,
		ContinuationToken: response.PaginationToken,
	}, nil
}
