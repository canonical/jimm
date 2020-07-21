// Copyright 2016 Canonical Ltd.

package jujuapi

import (
	"context"

	"gopkg.in/errgo.v1"
	"gopkg.in/mgo.v2/bson"

	"github.com/CanonicalLtd/jimm/internal/auth"
	"github.com/CanonicalLtd/jimm/internal/mongodoc"
	"github.com/CanonicalLtd/jimm/params"
)

// jimmV2 implements a facade V2 containing JIMM-specific API calls.
type jimmV2 struct {
	root *controllerRoot
}

// UserModelStats returns statistics about all the models that were created
// by the currently authenticated user.
func (j jimmV2) UserModelStats(ctx context.Context) (params.UserModelStatsResponse, error) {
	models := make(map[string]params.ModelStats)

	user := j.root.identity.Id()
	ctx = auth.ContextWithIdentity(ctx, j.root.identity)
	it := j.root.jem.DB.NewCanReadIter(ctx,
		j.root.jem.DB.Models().
			Find(bson.D{{"creator", user}}).
			Select(bson.D{{"uuid", 1}, {"path", 1}, {"creator", 1}, {"counts", 1}}).
			Iter())
	var model mongodoc.Model
	for it.Next(ctx, &model) {
		models[model.UUID] = params.ModelStats{
			Model:  userModelForModelDoc(&model),
			Counts: model.Counts,
		}
	}
	if err := it.Err(ctx); err != nil {
		return params.UserModelStatsResponse{}, errgo.Mask(err)
	}
	return params.UserModelStatsResponse{
		Models: models,
	}, nil
}

// DisableControllerUUIDMasking ensures that the controller UUID returned
// with any model information is the UUID of the juju controller that is
// hosting the model, and not JAAS.
func (j jimmV2) DisableControllerUUIDMasking(ctx context.Context) error {
	err := auth.CheckACL(ctx, j.root.identity, []string{string(j.root.jem.ControllerAdmin())})
	if err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrUnauthorized))
	}
	j.root.controllerUUIDMasking = false
	return nil
}

// ListControllers returns the list of juju controllers hosting models
// as part of this JAAS system.
func (j jimmV2) ListControllers(ctx context.Context) (params.ListControllerResponse, error) {
	ctx = auth.ContextWithIdentity(ctx, j.root.identity)

	err := auth.CheckACL(ctx, j.root.identity, []string{string(j.root.jem.ControllerAdmin())})
	if errgo.Cause(err) == params.ErrUnauthorized {
		// if the user isn't a controller admin return JAAS
		// itself as the only controller.

		srvVersion, err := j.root.jem.EarliestControllerVersion(ctx)
		if err != nil {
			return params.ListControllerResponse{}, errgo.Mask(err)
		}
		return params.ListControllerResponse{
			Controllers: []params.ControllerResponse{{
				Path:    params.EntityPath{User: "admin", Name: "jaas"},
				Public:  true,
				UUID:    j.root.params.ControllerUUID,
				Version: srvVersion.String(),
			}},
		}, nil
	}
	if err != nil {
		return params.ListControllerResponse{}, errgo.Mask(err)
	}

	var controllers []params.ControllerResponse
	iter := j.root.jem.DB.NewCanReadIter(ctx, j.root.jem.DB.Controllers().Find(nil).Sort("_id").Iter())
	defer iter.Close(ctx)
	var ctl mongodoc.Controller
	for iter.Next(ctx, &ctl) {
		controllers = append(controllers, params.ControllerResponse{
			Path:             ctl.Path,
			Public:           ctl.Public,
			UnavailableSince: newTime(ctl.UnavailableSince.UTC()),
			Location:         ctl.Location,
			UUID:             ctl.UUID,
			Version:          ctl.Version.String(),
		})
	}
	if err := iter.Err(ctx); err != nil {
		return params.ListControllerResponse{}, errgo.Notef(err, "cannot get controllers")
	}
	return params.ListControllerResponse{
		Controllers: controllers,
	}, nil
}
