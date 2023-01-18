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

	apiparams "github.com/CanonicalLtd/jimm/api/params"
	"github.com/CanonicalLtd/jimm/internal/db"
	"github.com/CanonicalLtd/jimm/internal/dbmodel"
	"github.com/CanonicalLtd/jimm/internal/errors"
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
		entry, err := db.GetGroup(ctx, trailer)
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
	const op = errors.Op("jujuapi.parseTag")
	tupleKeySplit := strings.SplitN(key, ":", 2)
	if len(tupleKeySplit) != 2 {
		return nil, "", errors.E(op, errors.CodeFailedToParseTupleKey, "tag does not have tuple key delimiter")
	}
	kind := tupleKeySplit[0]
	tagString := tupleKeySplit[1]
	tagString, relationSpecifier, err := resolveTupleObject(db, tagString)
	if err != nil {
		zapctx.Debug(ctx, "failed to resolve tuple object", zap.Error(err))
		return nil, "", errors.E(op, errors.CodeFailedToResolveTupleResource, err)
	}
	zapctx.Debug(ctx, "resolved JIMM tag", zap.String("tag", tagString), zap.String("relation-specifier", relationSpecifier))
	tag, err := jujuTagFromTuple(kind, tagString)
	return tag, relationSpecifier, err
}

// AddRelation creates a tuple between two objects [if applicable]
// within OpenFGA.
func (r *controllerRoot) AddRelation(ctx context.Context, req apiparams.AddRelationRequest) error {
	const op = errors.Op("jujuapi.AddRelation")
	db := r.jimm.Database
	ofc := r.ofgaClient

	parseTagError := func(msg string, key string, err error) error {
		zapctx.Debug(ctx, "failed to parse tuple key", zap.String("key", key), zap.Error(err))
		return errors.E(errors.Op("jujuapi.parseTag"), errors.CodeFailedToParseTupleKey, err, "failed to parse tuple user key: "+key)
	}

	keys := make([]openfga.TupleKey, 0, len(req.Tuples))
	for _, t := range req.Tuples {
		objectTag, objectTagRelationSpecifier, err := parseTag(ctx, db, t.Object)
		if err != nil {
			return parseTagError("failed to parse tuple user key", t.Object, err)
		}
		targetObjectTag, targetObjectTagRelationSpecifier, err := parseTag(ctx, db, t.TargetObject)
		if err != nil {
			return parseTagError("failed to parse tuple object key", t.TargetObject, err)
		}
		keys = append(
			keys,
			ofc.CreateTupleKey(
				objectTag.Kind()+":"+objectTag.Id()+objectTagRelationSpecifier,
				t.Relation,
				targetObjectTag.Kind()+":"+targetObjectTag.Id()+targetObjectTagRelationSpecifier,
			),
		)
	}
	if l := len(keys); l == 0 || l > 25 {
		return errors.E("length of" + strconv.Itoa(l) + "is not valid, please do not provide more than 25 tuple keys")
	}
	err := r.ofgaClient.AddRelations(ctx, keys...)
	if err != nil {
		zapctx.Error(ctx, "failed to add tuple(s)", zap.NamedError("add-relation-error", err))
		return errors.E(op, errors.CodeOpenFGARequestFailed, err)
	}
	return nil
}

// RemoveRelation removes a tuple between two objects [if applicable]
// within OpenFGA.
func (r *controllerRoot) RemoveRelation(ctx context.Context) error {
	return errors.E("Not implemented.")
}

// CheckRelation performs an authorisation check for a particular group/user tuple
// against another tuple within OpenFGA.
// This corresponds directly to /stores/{store_id}/check.
func (r *controllerRoot) CheckRelation(ctx context.Context, req apiparams.CheckRelationRequest) (apiparams.CheckRelationResponse, error) {
	const op = errors.Op("jujuapi.CheckRelation")
	checkResp := apiparams.CheckRelationResponse{Allowed: false}
	c := r.ofgaClient
	db := r.jimm.Database
	t := req.Tuple

	objectTag, objectTagRelationSpecifier, err := parseTag(ctx, db, t.Object)
	if err != nil {
		return checkResp, errors.E(op, err)
	}

	targetObjectTag, targetObjectTagRelationSpecifier, err := parseTag(ctx, db, t.TargetObject)
	if err != nil {
		return checkResp, errors.E(op, err)
	}

	allowed, resolution, err := c.CheckRelation(ctx, c.CreateTupleKey(
		objectTag.Kind()+":"+objectTag.Id()+objectTagRelationSpecifier,
		t.Relation,
		targetObjectTag.Kind()+":"+targetObjectTag.Id()+targetObjectTagRelationSpecifier,
	), false)
	if err != nil {
		zapctx.Error(ctx, "failed to check relation", zap.NamedError("check-relation-error", err))
		return checkResp, errors.E(op, errors.CodeOpenFGARequestFailed, err)
	}

	if allowed {
		checkResp.Allowed = allowed
	}
	zapctx.Debug(ctx, "check request", zap.String("allowed", strconv.FormatBool(allowed)), zap.String("reason", resolution))
	return checkResp, nil
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
