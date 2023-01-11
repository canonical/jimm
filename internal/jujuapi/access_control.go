package jujuapi

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	apiparams "github.com/CanonicalLtd/jimm/api/params"
	"github.com/CanonicalLtd/jimm/internal/db"
	"github.com/CanonicalLtd/jimm/internal/dbmodel"
	"github.com/CanonicalLtd/jimm/internal/errors"
	jimmnames "github.com/CanonicalLtd/jimm/pkg/names"
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

// RemoveGroup removes a group within JIMMs DB for reference by OpenFGA.
func (r *controllerRoot) RemoveGroup(ctx context.Context) error {
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

// ListGroup lists relational access control groups within JIMMs DB.
func (r *controllerRoot) ListGroups(ctx context.Context) error {
	return errors.E("Not implemented.")
}

// MapJIMMTagToJujuTag maps a JIMM tag [of any kind available] (i.e., controller-mycontroller:alex@external/mymodel.myoffer)
// into a juju tag (i.e., controller-<controller uuid>)
func MapJIMMTagToJujuTag(db db.Database, tag string) (string, error) {
	ctx := context.TODO()
	// Match upto -, and ignore all whitespace, lazy matches first -
	// and then subsequently in the second group matches all after -
	tagKindMatcher := regexp.MustCompile(`.*?([^\s][^-]*)(\-(.*?)\z)`)
	// Match upto : to denote a controller
	controllerMatcher := regexp.MustCompile(`.*?([^:|/]*)`)
	// Match between : and / to denate a controller user
	controllerUserMatcher := regexp.MustCompile(`\:(.*?)\/`)
	// Match upto . or end of line, in the case an application offer is present
	// despite being a model tag, denotes a model name
	controllerModelMatcher := regexp.MustCompile(`\/(.*?)([\.]|\z)`)
	// Match from . to EOL to denote an application offer
	applicationOfferMatcher := regexp.MustCompile(`\.(.*?)\z`)

	tagKindSubMatch := tagKindMatcher.FindStringSubmatch(tag)
	// Holds only the tag kind
	tagKind := tagKindSubMatch[1]
	// Holds the rest of the string after the tag kind
	trailer := tagKindSubMatch[3]

	switch tagKind {
	case names.UserTagKind:
		// Users are a different case, as we only care for the trailer
		userName := trailer
		zapctx.Debug(
			ctx,
			"mapping JIMM tags to Juju tags for tag kind: user",
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
		controller := dbmodel.Controller{Name: controllerName}
		zapctx.Debug(
			ctx,
			"mapping JIMM tags to Juju tags for tag kind: controller",
			zap.String("controller-name", controllerName),
		)
		err := db.GetController(ctx, &controller)
		if err != nil {
			return tag, errors.E("controller does not exist")
		}
		return names.ControllerTagKind + "-" + controller.UUID, nil
	case names.ModelTagKind:
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
			"mapping JIMM tags to Juju tags for tag kind: model",
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
		// This combination of ControllerID, OwnerUsername and ModelName *should* be universally unique
		// We query models separately as to not preload the models on the controller.
		model := dbmodel.Model{ControllerID: controller.ID, OwnerUsername: controllerUser, Name: modelName}
		err = db.GetModel(ctx, &model)
		if err != nil {
			return tag, errors.E("model not found")
		}
		return names.ModelTagKind + "-" + model.UUID.String, nil

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
			"mapping JIMM tags to Juju tags for tag kind: applicationoffer",
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

// AddRelation creates a tuple between two objects [if applicable]
// within OpenFGA.
func (r *controllerRoot) AddRelation(ctx context.Context, req apiparams.AddRelationRequest) error {
	const op = errors.Op("jujuapi.AddRelation")
	db := r.jimm.Database

	extractTag := func(key string) (names.Tag, error) {
		k := strings.Split(key, ":")
		objectType := k[0]
		objectId := k[1]
		err := errors.E(op, "could not determine tag type")
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

	ofc := r.ofgaClient
	keys := []openfga.TupleKey{}
	for _, t := range req.Tuples {
		userObject := ""
		// We manually validate the user object as OpenFGA doesn't currently run
		// the auth model against it to verify it is a valid object type.
		tag, err := extractTag(t.Object)
		if err != nil {
			return errors.E(op, fmt.Sprintf("failed to validate tag for object: %s", t.Object), err)
		}
		// Now we know the tag is valid, we can go ahead and format it into a valid OpenFGA
		// object type and id, however, the id provided by the user is resolved from NAME to
		// UUID bar actual users.
		switch t := tag.(type) {
		case names.UserTag:
			u := dbmodel.User{Username: t.Id()}
			// Get the user or create if necessary
			err := db.GetUser(ctx, &u)
			if err != nil {
				return errors.E(op, err)
			}
			// If all is OK, create tuple key
			userObject = fmt.Sprintf("%s:%s", "user", t.Name())
			if t.Domain() != "" {
				userObject = fmt.Sprintf("%s@%s", userObject, t.Domain())
			}

		case names.ModelTag:
			userObject = fmt.Sprintf("model:%s", t.Id())
			// m := dbmodel.Model{
			// 	UUID: sql.NullString{
			// 		String: t.Id(),
			// 		Valid:  true,
			// 	},
			// }
			// err = r.jimm.Database.GetModel(ctx, &m)
		case names.ControllerTag:
			userObject = fmt.Sprintf("controller:%s", t.Id())
		case names.ApplicationOfferTag:
			userObject = fmt.Sprintf("applicationoffer:%s", t.Id())
		case jimmnames.GroupTag:
			userObject = fmt.Sprintf("group:%s", t.Id())
		}

		keys = append(keys, ofc.CreateTupleKey(userObject, t.Relation, t.TargetObject))
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
