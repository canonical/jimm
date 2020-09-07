// Copyright 2016 Canonical Ltd.

package jujuapi

import (
	"context"

	"gopkg.in/errgo.v1"
	"gopkg.in/mgo.v2/bson"

	"github.com/CanonicalLtd/jimm/internal/auth"
	"github.com/CanonicalLtd/jimm/internal/jujuapi/rpc"
	"github.com/CanonicalLtd/jimm/internal/mongodoc"
	"github.com/CanonicalLtd/jimm/params"
)

func init() {
	facadeInit["JIMM"] = func(r *controllerRoot) []int {
		disableControllerUUIDMaskingMethod := rpc.Method(r.DisableControllerUUIDMasking)
		listControllersMethod := rpc.Method(r.ListControllers)
		userModelStatsMethod := rpc.Method(r.UserModelStats)

		r.AddMethod("JIMM", 1, "UserModelStats", userModelStatsMethod)

		r.AddMethod("JIMM", 2, "DisableControllerUUIDMasking", disableControllerUUIDMaskingMethod)
		r.AddMethod("JIMM", 2, "ListControllers", listControllersMethod)
		r.AddMethod("JIMM", 2, "UserModelStats", userModelStatsMethod)

		return []int{1, 2}
	}
}

// jimmV2 implements a facade V2 containing JIMM-specific API calls.
type jimmV2 struct {
	root *controllerRoot
}

// UserModelStats returns statistics about all the models that were created
// by the currently authenticated user.
func (r *controllerRoot) UserModelStats(ctx context.Context) (params.UserModelStatsResponse, error) {
	models := make(map[string]params.ModelStats)

	user := r.identity.Id()
	ctx = auth.ContextWithIdentity(ctx, r.identity)
	it := r.jem.DB.NewCanReadIter(ctx,
		r.jem.DB.Models().
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
func (r *controllerRoot) DisableControllerUUIDMasking(ctx context.Context) error {
	err := auth.CheckACL(ctx, r.identity, []string{string(r.jem.ControllerAdmin())})
	if err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrUnauthorized))
	}
	r.controllerUUIDMasking = false
	return nil
}

// ListControllers returns the list of juju controllers hosting models
// as part of this JAAS system.
func (r *controllerRoot) ListControllers(ctx context.Context) (params.ListControllerResponse, error) {
	ctx = auth.ContextWithIdentity(ctx, r.identity)

	err := auth.CheckACL(ctx, r.identity, []string{string(r.jem.ControllerAdmin())})
	if errgo.Cause(err) == params.ErrUnauthorized {
		// if the user isn't a controller admin return JAAS
		// itself as the only controller.

		srvVersion, err := r.jem.EarliestControllerVersion(ctx)
		if err != nil {
			return params.ListControllerResponse{}, errgo.Mask(err)
		}
		return params.ListControllerResponse{
			Controllers: []params.ControllerResponse{{
				Path:    params.EntityPath{User: "admin", Name: "jaas"},
				Public:  true,
				UUID:    r.params.ControllerUUID,
				Version: srvVersion.String(),
			}},
		}, nil
	}
	if err != nil {
		return params.ListControllerResponse{}, errgo.Mask(err)
	}

	var controllers []params.ControllerResponse
	iter := r.jem.DB.NewCanReadIter(ctx, r.jem.DB.Controllers().Find(nil).Sort("_id").Iter())
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
