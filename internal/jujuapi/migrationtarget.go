// Copyright 2025 Canonical.

package jujuapi

import (
	"context"
	"fmt"
	"time"

	"github.com/juju/description/v9"
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
		activate := rpc.Method(r.Activate)
		caCert := rpc.Method(r.CACert)
		adoptResources := rpc.Method(r.AdoptResources)
		checkMachines := rpc.Method(r.CheckMachines)
		importMethod := rpc.Method(r.Import)
		latestLogTime := rpc.Method(r.LatestLogTime)
		abort := rpc.Method(r.Abort)

		r.AddMethod("MigrationTarget", 4, "Prechecks", preChecks)
		r.AddMethod("MigrationTarget", 4, "CACert", caCert)
		r.AddMethod("MigrationTarget", 4, "Activate", activate)
		r.AddMethod("MigrationTarget", 4, "AdoptResources", adoptResources)
		r.AddMethod("MigrationTarget", 4, "Abort", abort)
		r.AddMethod("MigrationTarget", 4, "CheckMachines", checkMachines)
		r.AddMethod("MigrationTarget", 4, "Import", importMethod)
		r.AddMethod("MigrationTarget", 4, "LatestLogTime", latestLogTime)

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

func (r *controllerRoot) CheckMachines(ctx context.Context, args jujuparams.ModelArgs) (jujuparams.ErrorResults, error) {
	if !r.user.JimmAdmin {
		return jujuparams.ErrorResults{}, errors.E(errors.CodeUnauthorized, "unauthorized")
	}

	modelTag, err := names.ParseModelTag(args.ModelTag)
	if err != nil {
		return jujuparams.ErrorResults{}, errors.E(err)
	}

	results, err := r.jimm.JujuManager().CheckMachines(ctx, r.user, modelTag.Id())
	if err != nil {
		return jujuparams.ErrorResults{}, errors.E(err)
	}
	var errorResults []jujuparams.ErrorResult
	for _, result := range results {
		jujuError := jujuparams.Error{
			Message: result.Error(),
		}
		errorResults = append(errorResults, jujuparams.ErrorResult{Error: &jujuError})
	}

	return jujuparams.ErrorResults{Results: errorResults}, nil
}

// Abort implements the Abort method of the MigrationTarget facade.
// It is used by the source Juju controller to abort a model migration.
func (r *controllerRoot) Abort(ctx context.Context, args jujuparams.ModelArgs) error {
	if !r.user.JimmAdmin {
		return errors.E(errors.CodeUnauthorized, "unauthorized")
	}

	modelTag, err := names.ParseModelTag(args.ModelTag)
	if err != nil {
		return errors.E(err)
	}
	return r.jimm.JujuManager().AbortMigration(ctx, r.user, modelTag.Id())
}

// AdoptResources implements the AdoptResources method of the MigrationTarget facade.
// It is used by the source Juju controller to update the tags of the
// resources of a model that has been migrated to a new controller.
// This prevents the resources from being destroyed if the source controller
// is destroyed after the model is migrated away.
func (r *controllerRoot) AdoptResources(ctx context.Context, args jujuparams.AdoptResourcesArgs) error {
	if !r.user.JimmAdmin {
		return errors.E(errors.CodeUnauthorized, "unauthorized")
	}

	modelTag, err := names.ParseModelTag(args.ModelTag)
	if err != nil {
		return errors.E(err)
	}
	return r.jimm.JujuManager().AdoptResources(ctx, r.user, modelTag.Id(), args.SourceControllerVersion)
}

// LatestLogTime implements the LatestLogTime method of the MigrationTarget facade.
// It is used by the source Juju controller to retrieve the time of the
// latest log record it has seen.
func (r *controllerRoot) LatestLogTime(ctx context.Context, args jujuparams.ModelArgs) (time.Time, error) {
	if !r.user.JimmAdmin {
		return time.Time{}, errors.E(errors.CodeUnauthorized, "unauthorized")
	}

	modelTag, err := names.ParseModelTag(args.ModelTag)
	if err != nil {
		return time.Time{}, errors.E(err)
	}

	t, err := r.jimm.JujuManager().LatestLogTime(ctx, modelTag.Id())
	if err != nil {
		return time.Time{}, errors.E(err)
	}
	return t, nil
}

// Prechecks implements the Prechecks method of the MigrationTarget facade.
func (r *controllerRoot) Prechecks(ctx context.Context, args jujuparams.MigrationModelInfo) error {

	if !r.user.JimmAdmin {
		return errors.E(errors.CodeUnauthorized, "unauthorized")
	}

	ownerTag, err := names.ParseUserTag(args.OwnerTag)
	if err != nil {
		return errors.E(err)
	}

	modelDescription, err := description.Deserialize(args.ModelDescription)
	if err != nil {
		return errors.E(fmt.Errorf("failed to deserialize model description: %w", err))
	}

	model := migration.ModelInfo{
		UUID:                   args.UUID,
		Name:                   args.Name,
		Owner:                  ownerTag,
		AgentVersion:           args.AgentVersion,
		ControllerAgentVersion: args.ControllerAgentVersion,
		ModelDescription:       modelDescription,
	}
	err = r.jimm.JujuManager().Prechecks(ctx, r.user, model)
	if err != nil {
		return errors.E(err)
	}
	return nil
}

// Activate is the implementation of the Activate method of the MigrationTarget facade.
func (r *controllerRoot) Activate(ctx context.Context, args jujuparams.ActivateModelArgs) error {
	if !r.user.JimmAdmin {
		return errors.E(errors.CodeUnauthorized, "unauthorized")
	}

	modelTag, err := names.ParseModelTag(args.ModelTag)
	if err != nil {
		return errors.E(fmt.Errorf("invalid model tag: %w", err))
	}
	// The controller tag is optional, so we parse it only if provided.
	// It is only provided when args.CrossModelUUIDs is not empty.
	var controllerTag names.ControllerTag
	if args.ControllerTag != "" {
		controllerTag, err = names.ParseControllerTag(args.ControllerTag)
		if err != nil {

			return errors.E(fmt.Errorf("invalid controller tag: %w", err))
		}
	}

	err = r.jimm.JujuManager().Activate(
		ctx,
		modelTag, migration.SourceControllerInfo{
			ControllerTag:   controllerTag,
			ControllerAlias: args.ControllerAlias,
			Addrs:           args.SourceAPIAddrs,
			CACert:          args.SourceCACert,
		},
		args.CrossModelUUIDs)
	if err != nil {
		return err
	}
	return nil
}

// Import implements the Import method of the MigrationTarget facade.
// It imports resources into JIMM and proxies the import request to the target Juju controller.
func (r *controllerRoot) Import(ctx context.Context, serialized jujuparams.SerializedModel) error {
	if !r.user.JimmAdmin {
		return errors.E(errors.CodeUnauthorized, "unauthorized")
	}

	err := r.jimm.JujuManager().Import(ctx, r.user, serialized)
	if err != nil {
		return errors.E(fmt.Errorf("failed to import model: %w", err))
	}
	return nil
}
