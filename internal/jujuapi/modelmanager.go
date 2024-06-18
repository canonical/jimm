// Copyright 2016 Canonical Ltd.

package jujuapi

import (
	"context"
	"fmt"

	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v5"

	"github.com/canonical/jimm/internal/errors"
	"github.com/canonical/jimm/internal/jimm"
	"github.com/canonical/jimm/internal/jujuapi/rpc"
	"github.com/canonical/jimm/internal/servermon"
)

func init() {
	facadeInit["ModelManager"] = func(r *controllerRoot) []int {
		changeModelCredentialMethod := rpc.Method(r.ChangeModelCredential)
		createModelMethod := rpc.Method(r.CreateModel)
		destroyModelsMethod := rpc.Method(r.DestroyModels)
		dumpModelsMethod := rpc.Method(r.DumpModels)
		dumpModelsDBMethod := rpc.Method(r.DumpModelsDB)
		listModelSummariesMethod := rpc.Method(r.ListModelSummaries)
		listModelsMethod := rpc.Method(r.ListModels)
		modelInfoMethod := rpc.Method(r.ModelInfo)
		modelStatusMethod := rpc.Method(r.ModelStatus)
		modifyModelAccessMethod := rpc.Method(r.ModifyModelAccess)
		validateModelUpgradesMethod := rpc.Method(r.ValidateModelUpgrades)
		setModelDefaultsMethod := rpc.Method(r.SetModelDefaults)
		unsetModelDefaultsMethod := rpc.Method(r.UnsetModelDefaults)
		modelDefaultsForCloudsMethod := rpc.Method(r.ModelDefaultsForClouds)

		r.AddMethod("ModelManager", 9, "ChangeModelCredential", changeModelCredentialMethod)
		r.AddMethod("ModelManager", 9, "CreateModel", createModelMethod)
		r.AddMethod("ModelManager", 9, "DestroyModels", destroyModelsMethod)
		r.AddMethod("ModelManager", 9, "DumpModels", dumpModelsMethod)
		r.AddMethod("ModelManager", 9, "DumpModelsDB", dumpModelsDBMethod)
		r.AddMethod("ModelManager", 9, "ListModelSummaries", listModelSummariesMethod)
		r.AddMethod("ModelManager", 9, "ListModels", listModelsMethod)
		r.AddMethod("ModelManager", 9, "ModelInfo", modelInfoMethod)
		r.AddMethod("ModelManager", 9, "ModelStatus", modelStatusMethod)
		r.AddMethod("ModelManager", 9, "ModifyModelAccess", modifyModelAccessMethod)
		r.AddMethod("ModelManager", 9, "ValidateModelUpgrades", validateModelUpgradesMethod)
		r.AddMethod("ModelManager", 9, "SetModelDefaults", setModelDefaultsMethod)
		r.AddMethod("ModelManager", 9, "UnsetModelDefaults", unsetModelDefaultsMethod)
		r.AddMethod("ModelManager", 9, "ModelDefaultsForClouds", modelDefaultsForCloudsMethod)

		return []int{9}
	}
}

// DumpModels implements the DumpModels method of the modelmanager (version
// 3 onwards) facade. The model dump is passed back as-is from the
// controller without any changes from JIMM.
func (r *controllerRoot) DumpModels(ctx context.Context, args jujuparams.DumpModelRequest) jujuparams.StringResults {
	const op = errors.Op("jujuapi.DumpModels")

	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	results := make([]jujuparams.StringResult, len(args.Entities))
	for i, ent := range args.Entities {
		mt, err := names.ParseModelTag(ent.Tag)
		if err != nil {
			results[i].Error = mapError(errors.E(op, err, errors.CodeBadRequest))
		}
		results[i].Result, err = r.jimm.DumpModel(ctx, r.user, mt, args.Simplified)
		if err != nil {
			results[i].Error = mapError(errors.E(op, err))
		}
	}
	return jujuparams.StringResults{
		Results: results,
	}
}

// ListModelSummaries returns summaries for all the models that that
// authenticated user has access to. The request parameter is ignored.
func (r *controllerRoot) ListModelSummaries(ctx context.Context, _ jujuparams.ModelSummariesRequest) (jujuparams.ModelSummaryResults, error) {
	const op = errors.Op("jujuapi.ListModelSummaries")

	summaries, err := r.jimm.GetAllModelSummariesForUser(ctx, r.user)
	if err != nil {
		return summaries, err
	}

	// If controller masking is set, don't reveal the underlying controllers UUID
	// when performing a summary and instead set JIMM's controller ID for each.
	if r.controllerUUIDMasking {
		for _, results := range summaries.Results {
			ms := results.Result
			ms.ControllerUUID = r.params.ControllerUUID
		}
	}

	// Return the masked summaries from all underlying controllers.
	return summaries, err
}

// ListModels returns the models that the authenticated user
// has access to. The user parameter is ignored.
func (r *controllerRoot) ListModels(ctx context.Context, _ jujuparams.Entity) (jujuparams.UserModelList, error) {
	return r.allModels(ctx)
}

// ModelInfo implements the ModelManager facade's ModelInfo method.
func (r *controllerRoot) ModelInfo(ctx context.Context, args jujuparams.Entities) (jujuparams.ModelInfoResults, error) {
	const op = errors.Op("jujuapi.ModelInfo")

	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	results := make([]jujuparams.ModelInfoResult, len(args.Entities))
	for i, arg := range args.Entities {
		mt, err := names.ParseModelTag(arg.Tag)
		if err != nil {
			results[i].Error = mapError(errors.E(op, err, errors.CodeBadRequest))
			continue
		}
		results[i].Result, err = r.jimm.ModelInfo(ctx, r.user, mt)
		if err != nil {
			if errors.ErrorCode(err) == errors.CodeNotFound {
				// Map not-found errors to unauthorized, this is what juju
				// does.
				err = errors.E(op, errors.CodeUnauthorized, "unauthorized")
			}
			results[i].Error = mapError(errors.E(op, err))
		} else {
			if r.controllerUUIDMasking {
				results[i].Result.ControllerUUID = r.params.ControllerUUID
			}
		}
	}
	return jujuparams.ModelInfoResults{
		Results: results,
	}, nil
}

// CreateModel implements the ModelManager facade's CreateModel method.
func (r *controllerRoot) CreateModel(ctx context.Context, args jujuparams.ModelCreateArgs) (jujuparams.ModelInfo, error) {
	const op = errors.Op("jujuapi.CreateModel")

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var mca jimm.ModelCreateArgs
	if err := mca.FromJujuModelCreateArgs(&args); err != nil {
		return jujuparams.ModelInfo{}, errors.E(op, err)
	}
	info, err := r.jimm.AddModel(ctx, r.user, &mca)
	if err != nil {
		servermon.ModelsCreatedFailCount.Inc()
		return jujuparams.ModelInfo{}, errors.E(op, err)
	}

	servermon.ModelsCreatedCount.Inc()
	if r.controllerUUIDMasking {
		info.ControllerUUID = r.params.ControllerUUID
	}
	return *info, nil
}

// DestroyModels implements the ModelManager facade's DestroyModels
// method used in version 4 onwards.
func (r *controllerRoot) DestroyModels(ctx context.Context, args jujuparams.DestroyModelsParams) (jujuparams.ErrorResults, error) {
	const op = errors.Op("jujuapi.DestroyModel")

	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	results := make([]jujuparams.ErrorResult, len(args.Models))

	for i, model := range args.Models {
		mt, err := names.ParseModelTag(model.ModelTag)
		if err != nil {
			results[i].Error = mapError(errors.E(op, err, errors.CodeBadRequest))
			continue
		}

		if err := r.jimm.DestroyModel(ctx, r.user, mt, model.DestroyStorage, model.Force, model.MaxWait, model.Timeout); err != nil {
			if errors.ErrorCode(err) != errors.CodeNotFound {
				// It isn't an error to try and destroy an already
				// destroyed model.
				results[i].Error = mapError(errors.E(op, err))
			}
		}
	}

	return jujuparams.ErrorResults{
		Results: results,
	}, nil
}

// ModifyModelAccess implements the ModelManager facade's ModifyModelAccess method.
func (r *controllerRoot) ModifyModelAccess(ctx context.Context, args jujuparams.ModifyModelAccessRequest) (jujuparams.ErrorResults, error) {
	const op = errors.Op("jujuapi.ModifyModelAccess")

	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	results := make([]jujuparams.ErrorResult, len(args.Changes))
	for i, change := range args.Changes {
		mt, err := names.ParseModelTag(change.ModelTag)
		if err != nil {
			results[i].Error = mapError(errors.E(op, err, errors.CodeBadRequest))
			continue
		}
		user, err := parseUserTag(change.UserTag)
		if err != nil {
			results[i].Error = mapError(errors.E(op, err, errors.CodeBadRequest))
			continue
		}
		switch change.Action {
		case jujuparams.GrantModelAccess:
			err = r.jimm.GrantModelAccess(ctx, r.user, mt, user, change.Access)
		case jujuparams.RevokeModelAccess:
			err = r.jimm.RevokeModelAccess(ctx, r.user, mt, user, change.Access)
		default:
			err = errors.E(op, errors.CodeBadRequest, fmt.Sprintf("invalid action %q", change.Action))
		}
		if errors.ErrorCode(err) == errors.CodeNotFound {
			err = errors.E(op, errors.CodeUnauthorized, "unauthorized")
		}
		results[i].Error = mapError(err)
	}
	return jujuparams.ErrorResults{
		Results: results,
	}, nil
}

// DumpModelsDB implements the modelmanager facades DumpModelsDB API. The
// model dump is passed back as-is from the controller without any
// changes from JIMM.
func (r *controllerRoot) DumpModelsDB(ctx context.Context, args jujuparams.Entities) jujuparams.MapResults {
	const op = errors.Op("jujuapi.DumpModelsDB")

	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	results := make([]jujuparams.MapResult, len(args.Entities))
	for i, ent := range args.Entities {
		mt, err := names.ParseModelTag(ent.Tag)
		if err != nil {
			results[i].Error = mapError(errors.E(op, err, errors.CodeBadRequest))
		}
		results[i].Result, err = r.jimm.DumpModelDB(ctx, r.user, mt)
		if err != nil {
			results[i].Error = mapError(errors.E(op, err))
		}
	}
	return jujuparams.MapResults{
		Results: results,
	}
}

// ChangeModelCredential implements the ModelManager (v5) facade's
// ChangeModelCredential method.
func (r *controllerRoot) ChangeModelCredential(ctx context.Context, args jujuparams.ChangeModelCredentialsParams) (jujuparams.ErrorResults, error) {
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	results := make([]jujuparams.ErrorResult, len(args.Models))
	for i, arg := range args.Models {
		results[i].Error = mapError(r.changeModelCredential(ctx, arg))
	}
	return jujuparams.ErrorResults{
		Results: results,
	}, nil
}

func (r *controllerRoot) changeModelCredential(ctx context.Context, arg jujuparams.ChangeModelCredentialParams) error {
	const op = errors.Op("jujuapi.ChangeModelCredential")

	mt, err := names.ParseModelTag(arg.ModelTag)
	if err != nil {
		return errors.E(op, err, errors.CodeBadRequest)
	}
	cct, err := names.ParseCloudCredentialTag(arg.CloudCredentialTag)
	if err != nil {
		return errors.E(op, err, errors.CodeBadRequest)
	}
	if err := r.jimm.ChangeModelCredential(ctx, r.user, mt, cct); err != nil {
		return errors.E(op, err)
	}
	return nil
}

// ValidateModelUpgrades validates if a model is allowed to perform an upgrade.
// Examples of why you would want to block a model upgrade, would be situations
// like upgrade-series. If performing an upgrade-series we don't know the
// current status of the machine, so performing an upgrade-model can lead to
// bad unintended errors down the line.
func (r *controllerRoot) ValidateModelUpgrades(ctx context.Context, args jujuparams.ValidateModelUpgradeParams) (jujuparams.ErrorResults, error) {
	const op = errors.Op("jujuapi.ValidateModelUpgrades")

	results := make([]jujuparams.ErrorResult, len(args.Models))
	for i, arg := range args.Models {
		modelTag, err := names.ParseModelTag(arg.ModelTag)
		if err != nil {
			results[i].Error = mapError(errors.E(op, err, errors.CodeBadRequest))
			continue
		}
		results[i].Error = mapError(r.jimm.ValidateModelUpgrade(ctx, r.user, modelTag, args.Force))
	}
	return jujuparams.ErrorResults{
		Results: results,
	}, nil
}

// SetModelDefaults writes new values for the specified default model settings.
func (r *controllerRoot) SetModelDefaults(ctx context.Context, args jujuparams.SetModelDefaults) (jujuparams.ErrorResults, error) {
	const op = errors.Op("jujuapi.ModelDefaultsForClouds")

	results := make([]jujuparams.ErrorResult, len(args.Config))
	for i, config := range args.Config {
		cloudTag, err := names.ParseCloudTag(config.CloudTag)
		if err != nil {
			results[i].Error = mapError(errors.E(op, err))
			continue
		}
		results[i].Error = mapError(r.jimm.SetModelDefaults(ctx, r.user.Identity, cloudTag, config.CloudRegion, config.Config))
	}

	return jujuparams.ErrorResults{
		Results: results,
	}, nil
}

// UnsetModelDefaults removes the specified default model settings.
func (r *controllerRoot) UnsetModelDefaults(ctx context.Context, args jujuparams.UnsetModelDefaults) (jujuparams.ErrorResults, error) {
	results := make([]jujuparams.ErrorResult, len(args.Keys))
	for i, key := range args.Keys {
		cloudTag, err := names.ParseCloudTag(key.CloudTag)
		if err != nil {
			results[i].Error = mapError(err)
			continue
		}
		results[i].Error = mapError(r.jimm.UnsetModelDefaults(ctx, r.user.Identity, cloudTag, key.CloudRegion, key.Keys))
	}

	return jujuparams.ErrorResults{
		Results: results,
	}, nil
}

// ModelDefaultsForClouds returns the default config values for the specified
// clouds.
func (r *controllerRoot) ModelDefaultsForClouds(ctx context.Context, args jujuparams.Entities) (jujuparams.ModelDefaultsResults, error) {
	const op = errors.Op("jujuapi.ModelDefaultsForClouds")

	result := jujuparams.ModelDefaultsResults{}
	result.Results = make([]jujuparams.ModelDefaultsResult, len(args.Entities))
	for i, entity := range args.Entities {
		cloudTag, err := names.ParseCloudTag(entity.Tag)
		if err != nil {
			result.Results[i].Error = mapError(errors.E(op, err))
			continue
		}
		defaults, err := r.jimm.ModelDefaultsForCloud(ctx, r.user.Identity, cloudTag)
		if err != nil {
			result.Results[i].Error = mapError(errors.E(op, err))
			continue
		}
		result.Results[i] = defaults
	}
	return result, nil
}

// TODO (ashipika) Implement ModelDefaults - need to determine which cloud this would refer to.
