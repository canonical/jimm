package jujuapi

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	apiparams "github.com/CanonicalLtd/jimm/api/params"
	"github.com/CanonicalLtd/jimm/internal/db"
	"github.com/CanonicalLtd/jimm/internal/dbmodel"
	"github.com/CanonicalLtd/jimm/internal/errors"
	jimmnames "github.com/CanonicalLtd/jimm/pkg/names"
	"github.com/google/uuid"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/names/v4"
	"github.com/juju/zaputil"
	"github.com/juju/zaputil/zapctx"
	openfga "github.com/openfga/go-sdk"
	"go.uber.org/zap"
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

// ResolveTupleObject resolves JIMM tag [of any kind available] (i.e., controller-mycontroller:alex@external/mymodel.myoffer)
// into a juju string tag (i.e., controller-<controller uuid>).
//
// If the JIMM tag is aleady of juju string tag form, the transformation is left alone.
//
// In both cases though, the resource the tag pertains to is validated to exist within the database.
func ResolveTupleObject(db db.Database, tag string) (string, error) {
	ctx := context.Background()
	tagKindMatcher := regexp.MustCompile(`.*?([^\s][^-]*)(\-(.*?)\z)`)
	controllerMatcher := regexp.MustCompile(`.*?([^:|/]*)`)
	controllerUserMatcher := regexp.MustCompile(`\:(.*?)\/`)
	controllerModelMatcher := regexp.MustCompile(`\/(.*?)([\.]|\z)`)
	applicationOfferMatcher := regexp.MustCompile(`\.(.*?)\z`)

	tagKindSubMatch := tagKindMatcher.FindStringSubmatch(tag)
	tagKind := tagKindSubMatch[1]
	trailer := tagKindSubMatch[3]

	switch tagKind {
	case names.UserTagKind:
		// Users are a different case, as we only care for the trailer
		userName := trailer
		zapctx.Debug(
			ctx,
			"Resolving JIMM tags to Juju tags for tag kind: user",
			zap.String("user-name", userName),
		)
		return names.UserTagKind + "-" + trailer, nil

	case jimmnames.GroupTagKind:
		// TODO(ale8k): Ask ales why group has no UUID?
		// Also why it returns the entry rather than puts the entry in a pointer
		entry, err := db.GetGroup(ctx, trailer)
		if err != nil {
			return tag, errors.E("user group does not exist")
		}
		return jimmnames.GroupTagKind + "-" + entry.Name, nil
	case names.ControllerTagKind:
		controllerName := controllerMatcher.FindString(trailer)
		cuuid := ""

		if _, err := uuid.Parse(trailer); err != nil {
			controller := dbmodel.Controller{Name: controllerName}
			zapctx.Debug(
				ctx,
				"Resolving JIMM tags to Juju tags for tag kind: controller",
				zap.String("controller-name", controllerName),
			)
			err := db.GetController(ctx, &controller)
			if err != nil {
				return tag, errors.E("controller does not exist")
			}
			cuuid = controller.UUID
		} else {
			controller := dbmodel.Controller{UUID: trailer}
			zapctx.Debug(
				ctx,
				"Tag appears to already be a UUID: controller",
				zap.String("controller-name", trailer),
			)
			err := db.GetController(ctx, &controller)
			if err != nil {
				return tag, errors.E("controller does not exist")
			}
			cuuid = controller.UUID
		}

		return names.ControllerTagKind + "-" + cuuid, nil
	case names.ModelTagKind:
		// Determine if this is a UUID or JIMM tag
		muuid := ""

		if _, err := uuid.Parse(trailer); err != nil {

			var sm []string
			var sm2 []string
			if sm = controllerUserMatcher.FindStringSubmatch(trailer); len(sm) < 2 {
				return tag, errors.E("could not find controller user in tag")
			}
			if sm2 = controllerModelMatcher.FindStringSubmatch(trailer); len(sm2) < 2 {
				return tag, errors.E("could not find controller model in tag")
			}
			controllerName := controllerMatcher.FindString(trailer)
			controllerUser := sm[1]
			modelName := sm2[1]
			zapctx.Debug(
				ctx,
				"Resolving JIMM tags to Juju tags for tag kind: model",
				zap.String("controller-name", controllerName),
				zap.String("controller-user", controllerUser),
				zap.String("model-name", modelName),
			)

			// We only care about the controller ID to ensure
			// an absolute unique combination when querying
			// the model
			controller := dbmodel.Controller{Name: controllerName}
			err := db.GetController(ctx, &controller)
			if err != nil {
				return tag, errors.E("controller does not exist")
			}
			// TODO(ale8k): Investigate why when querying with model name as emptry string
			// it's somehow getting the model anyway...
			if modelName == "" {
				return tag, errors.E("model not found")
			}
			// This combination of ControllerID, OwnerUsername and ModelName *should* be universally unique
			// We query models separately as to not preload the models on the controller.
			model := dbmodel.Model{ControllerID: controller.ID, OwnerUsername: controllerUser, Name: modelName}
			err = db.GetModel(ctx, &model)
			if err != nil {
				return tag, errors.E("model not found")
			}
			muuid = model.UUID.String
		} else {
			model := dbmodel.Model{UUID: sql.NullString{String: trailer, Valid: true}}
			zapctx.Debug(
				ctx,
				"Tag appears to already be a UUID: model",
				zap.String("model-name", trailer),
			)
			err = db.GetModel(ctx, &model)
			if err != nil {
				zapctx.Debug(ctx, "?????????????????????????????????????????")
				return tag, errors.E("model not found")
			}
			muuid = model.UUID.String
		}
		return names.ModelTagKind + "-" + muuid, nil

	case names.ApplicationOfferTagKind:
		var sm []string
		var sm2 []string
		var sm3 []string
		if sm = controllerUserMatcher.FindStringSubmatch(trailer); len(sm) < 2 {
			return tag, errors.E("could not find controller user in tag")
		}
		if sm2 = controllerModelMatcher.FindStringSubmatch(trailer); len(sm2) < 2 {
			return tag, errors.E("could not find controller model in tag")
		}
		if sm3 = applicationOfferMatcher.FindStringSubmatch(trailer); len(sm2) < 2 {
			return tag, errors.E("could not find application offer in tag")
		}
		controllerName := controllerMatcher.FindString(trailer)
		controllerUser := sm[1]
		modelName := sm2[1]
		applicationOfferName := sm3[1]
		zapctx.Debug(
			ctx,
			"Resolving JIMM tags to Juju tags for tag kind: applicationoffer",
			zap.String("controller-name", controllerName),
			zap.String("controller-user", controllerUser),
			zap.String("model-name", modelName),
			zap.String("application-offer-name", applicationOfferName),
		)
		// This is simply for validation purposes and to be 100% certain
		offerURL, err := crossmodel.ParseOfferURL(trailer)
		if err != nil {
			return tag, errors.E("failed to parse offer url")
		}
		offer := dbmodel.ApplicationOffer{URL: offerURL.String()}
		err = db.GetApplicationOffer(ctx, &offer)
		if err != nil {
			return tag, errors.E("applicationoffer not found")
		}
		return names.ApplicationOfferTagKind + "-" + offer.UUID, nil
	}
	return tag, errors.E("failed to map tag")
}

// ParseJujuTag attempts to parse the provided key
// into a juju tag, and returns an error if this is not possible.
func MapTupleObjectToJujuTag(objectType string, objectId string) (names.Tag, error) {
	err := errors.E("could not determine tag type")
	switch objectType {
	case names.UserTagKind:
		return names.ParseUserTag(objectId)
	case names.ModelTagKind:
		return names.ParseModelTag(objectId)
	case names.ControllerTagKind:
		return names.ParseControllerTag(objectId)
	case names.ApplicationOfferTagKind:
		return names.ParseApplicationOfferTag(objectId)
	case jimmnames.GroupTagKind:
		return jimmnames.ParseGroupTag(objectId)
	default:
		return nil, err
	}
}

// ParseTag attempts to parse the provided key into a tag whilst additionally
// ensuring the resource exists for said tag.
//
// This key may be in the form of either a JIMM tag string or Juju tag string.
func ParseTag(db db.Database, key string) (names.Tag, error) {
	ctx := context.Background()
	tupleKeySplit := strings.SplitN(key, ":", 2)
	if len(tupleKeySplit) != 2 {
		return nil, errors.E("tag does not have tuple key delimiter")
	}
	kind := tupleKeySplit[0]
	tagString := tupleKeySplit[1]
	tagString, err := ResolveTupleObject(db, tagString)
	if err != nil {
		zapctx.Debug(ctx, "failed to resolve tuple object", zap.Error(err))
		return nil, errors.E("failed to resolve tuple object: " + err.Error())
	}
	return MapTupleObjectToJujuTag(kind, tagString)
}

// AddRelation creates a tuple between two objects [if applicable]
// within OpenFGA.
func (r *controllerRoot) AddRelation(ctx context.Context, req apiparams.AddRelationRequest) error {
	const op = errors.Op("jujuapi.AddRelation")
	db := r.jimm.Database

	ofc := r.ofgaClient
	keys := []openfga.TupleKey{}
	for _, t := range req.Tuples {
		objectTag, err := ParseTag(db, t.Object)
		if err != nil {
			zapctx.Debug(ctx, "failed to parse tuple user key", zap.String("key", t.Object), zap.Error(err))
			return errors.E("failed to parse tuple user key: " + t.Object)
		}
		targetObject, err := ParseTag(db, t.TargetObject)
		if err != nil {
			zapctx.Debug(ctx, "failed to parse tuple object key", zap.String("key", t.TargetObject), zap.Error(err))
			return errors.E("failed to parse tuple object key: " + t.TargetObject)
		}
		keys = append(
			keys,
			ofc.CreateTupleKey(
				fmt.Sprintf("%s:%s", objectTag.Kind(), objectTag.Id()),
				t.Relation,
				fmt.Sprintf("%s:%s", targetObject.Kind(), targetObject.Id()),
			),
		)
	}
	if l := len(keys); l == 0 || l > 25 {
		return errors.E("length of" + strconv.Itoa(l) + "is not valid, please do not provide more than 25 tuple keys")
	}
	err := r.ofgaClient.AddRelations(ctx, keys...)
	if err != nil {
		zapctx.Error(ctx, "failed to add tuple(s)", zap.NamedError("add-relation-error", err))
		return errors.E(op, err)
	}
	return nil
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
