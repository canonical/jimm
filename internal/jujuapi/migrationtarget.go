// Copyright 2025 Canonical.

package jujuapi

import (
	"context"

	"github.com/juju/juju/core/migration"
	"github.com/juju/juju/rpc/params"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v5"

	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/jujuapi/rpc"
)

/*
Below are the remaining RPC methods to add to the MigrationTarget facade.
There are additional HTTP endpoints not included here that also need to be implemented.

func (api *API) Abort(args params.ModelArgs) error
func (api *API) Activate(args params.ActivateModelArgs) error
func (api *API) AdoptResources(args params.AdoptResourcesArgs) error
func (api *API) CACert() (params.BytesResult, error)
func (api *API) CheckMachines(args params.ModelArgs) (params.ErrorResults, error)
func (api *API) Import(serialized params.SerializedModel) error
func (api *API) LatestLogTime(args params.ModelArgs) (time.Time, error)
func (api *API) Prechecks(model params.MigrationModelInfo) error
*/

func init() {
	facadeInit["MigrationTarget"] = func(r *controllerRoot) []int {
		preChecks := rpc.Method(r.Prechecks)
		caCert := rpc.Method(r.CACert)
		adoptResources := rpc.Method(r.AdoptResources)

		r.AddMethod("MigrationTarget", 4, "Prechecks", preChecks)
		r.AddMethod("MigrationTarget", 4, "CACert", caCert)
		r.AddMethod("MigrationTarget", 4, "AdoptResources", adoptResources)

		return []int{4}
	}
}

// CACert implements the CACert method of the MigrationTarget facade.
// It is used by the source Juju controller to retrieve the CA cert of
// the target controller during model migration, if the client did not
// send a CA cert to the source controller (possible if the controller
// uses a public CA rather than a self-signed cert).
//
// The above is nonsensical because if the source controller can reach
// the target controller (and because Juju enforces WSS), it already has
// everything it needs i.e. it either has the self-signed CA cert or it
// was able to connect thanks to a public CA.
//
// However, because the source controller requires this call to be successful,
// but doesn't actually require the result to have len() > 0, we can return an empty result.
func (r *controllerRoot) CACert() (jujuparams.BytesResult, error) {
	return jujuparams.BytesResult{}, nil
}

// AdoptResources implements the AdoptResources method of the MigrationTarget facade.
// It is used by the source Juju controller to update the tags of the
// resources of a model that has been migrated to a new controller.
// This prevents the resources from being destroyed if the source controller
// is destroyed after the model is migrated away.
func (r *controllerRoot) AdoptResources(ctx context.Context, args params.AdoptResourcesArgs) error {
	const op = errors.Op("jujuapi.AdoptResources")

	if !r.user.JimmAdmin {
		return errors.E(op, errors.CodeUnauthorized, "unauthorized")
	}

	modelTag, err := names.ParseModelTag(args.ModelTag)
	if err != nil {
		return errors.E(op, err)
	}
	return r.jimm.JujuManager().AdoptResources(ctx, r.user, modelTag.Id(), args.SourceControllerVersion)
}

// Prechecks implements the Prechecks method of the MigrationTarget facade.
func (r *controllerRoot) Prechecks(ctx context.Context, args jujuparams.MigrationModelInfo) error {
	const op = errors.Op("jujuapi.Prechecks")

	if !r.user.JimmAdmin {
		return errors.E(op, errors.CodeUnauthorized, "unauthorized")
	}

	ownerTag, err := names.ParseUserTag(args.OwnerTag)
	if err != nil {
		return errors.E(op, err)
	}
	model := migration.ModelInfo{
		UUID:                   args.UUID,
		Name:                   args.Name,
		Owner:                  ownerTag,
		AgentVersion:           args.AgentVersion,
		ControllerAgentVersion: args.ControllerAgentVersion,
	}
	err = r.jimm.JujuManager().Prechecks(ctx, r.user, model)
	if err != nil {
		return errors.E(op, err)
	}
	return nil
}
