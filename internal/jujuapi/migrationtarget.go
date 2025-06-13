// Copyright 2025 Canonical.

package jujuapi

import (
	"context"

	"github.com/juju/juju/core/migration"
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

		r.AddMethod("MigrationTarget", 4, "Prechecks", preChecks)

		return []int{4}
	}
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
