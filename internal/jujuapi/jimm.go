// Copyright 2016 Canonical Ltd.

package jujuapi

import (
	"context"

	"gopkg.in/errgo.v1"

	"github.com/CanonicalLtd/jimm/internal/auth"
	"github.com/CanonicalLtd/jimm/internal/jujuapi/rpc"
	"github.com/CanonicalLtd/jimm/internal/mongodoc"
	"github.com/CanonicalLtd/jimm/params"
	jujuparams "github.com/juju/juju/apiserver/params"
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
	err := r.jem.ForEachModel(ctx, r.identity, jujuparams.ModelReadAccess, func(m *mongodoc.Model) error {
		if m.Creator != r.identity.Id() {
			return nil
		}
		models[m.UUID] = params.ModelStats{
			Model:  userModelForModelDoc(m),
			Counts: m.Counts,
		}
		return nil
	})
	if err != nil {
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
	err := auth.CheckACL(ctx, r.identity, []string{string(r.jem.ControllerAdmin())})
	if errgo.Cause(err) == params.ErrUnauthorized {
		// if the user isn't a controller admin return JAAS
		// itself as the only controller.

		srvVersion, err := r.jem.EarliestControllerVersion(ctx, r.identity)
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
	err = r.jem.ForEachController(ctx, r.identity, func(ctl *mongodoc.Controller) error {
		controllers = append(controllers, params.ControllerResponse{
			Path:             ctl.Path,
			Public:           ctl.Public,
			UnavailableSince: newTime(ctl.UnavailableSince.UTC()),
			Location:         ctl.Location,
			UUID:             ctl.UUID,
			Version:          ctl.Version.String(),
		})
		return nil
	})
	if err != nil {
		return params.ListControllerResponse{}, errgo.Mask(err)
	}
	return params.ListControllerResponse{
		Controllers: controllers,
	}, nil
}
