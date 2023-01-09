package jujuapi

import (
	"context"
	"fmt"
	"strings"

	apiparams "github.com/CanonicalLtd/jimm/api/params"
	"github.com/CanonicalLtd/jimm/internal/errors"
	jimmnames "github.com/CanonicalLtd/jimm/pkg/names"
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

// AddRelation creates a tuple between two objects [if applicable]
// within OpenFGA.
func (r *controllerRoot) AddRelation(ctx context.Context, req apiparams.AddRelationRequest) error {
	const op = errors.Op("jujuapi.AddRelation")

	extractTag := func(key string) (names.Tag, error) {
		k := strings.Split(key, ":")
		objectType := k[0]
		objectId := k[1]
		err := errors.E(op, "could not determine tag type")
		switch objectType {
		case "user":
			return names.ParseUserTag(objectId)
		case "model":
			return names.ParseModelTag(objectId)
		case "controller":
			return names.ParseControllerTag(objectId)
		case "applicationoffer":
			return names.ParseApplicationTag(objectId)
		case "group":
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
		// object type and id
		switch t := tag.(type) {
		case names.UserTag:
			userObject = fmt.Sprintf("%s:%s", "user", t.Name())
			if t.Domain() != "" {
				userObject = fmt.Sprintf("%s@%s", userObject, t.Domain())
			}
		case names.ModelTag:
			userObject = fmt.Sprintf("model:%s", t.Id())
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
