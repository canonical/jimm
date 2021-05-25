// Copyright 2016 Canonical Ltd.

package jujuapi

import (
	"context"
	"fmt"

	jujuparams "github.com/juju/juju/apiserver/params"
	"github.com/juju/names/v4"

	"github.com/CanonicalLtd/jimm/internal/dbmodel"
	"github.com/CanonicalLtd/jimm/internal/errors"
	"github.com/CanonicalLtd/jimm/internal/jimm"
	"github.com/CanonicalLtd/jimm/internal/jujuapi/rpc"
	"github.com/CanonicalLtd/jimm/internal/servermon"
)

func init() {
	facadeInit["ModelManager"] = func(r *controllerRoot) []int {
		changeModelCredentialMethod := rpc.Method(r.ChangeModelCredential)
		createModelMethod := rpc.Method(r.CreateModel)
		destroyModelsMethod := rpc.Method(r.DestroyModels)
		destroyModelsV4Method := rpc.Method(r.DestroyModelsV4)
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

		r.AddMethod("ModelManager", 2, "CreateModel", createModelMethod)
		r.AddMethod("ModelManager", 2, "DestroyModels", destroyModelsMethod)
		r.AddMethod("ModelManager", 2, "DumpModelsDB", dumpModelsDBMethod)
		r.AddMethod("ModelManager", 2, "ListModels", listModelsMethod)
		r.AddMethod("ModelManager", 2, "ModelStatus", modelStatusMethod)
		r.AddMethod("ModelManager", 2, "ModifyModelAccess", modifyModelAccessMethod)
		r.AddMethod("ModelManager", 2, "SetModelDefaults", setModelDefaultsMethod)
		r.AddMethod("ModelManager", 2, "UnsetModelDefaults", unsetModelDefaultsMethod)

		r.AddMethod("ModelManager", 3, "CreateModel", createModelMethod)
		r.AddMethod("ModelManager", 3, "DestroyModels", destroyModelsMethod)
		r.AddMethod("ModelManager", 3, "DumpModels", dumpModelsMethod)
		r.AddMethod("ModelManager", 3, "DumpModelsDB", dumpModelsDBMethod)
		r.AddMethod("ModelManager", 3, "ListModels", listModelsMethod)
		r.AddMethod("ModelManager", 3, "ModelInfo", modelInfoMethod)
		r.AddMethod("ModelManager", 3, "ModelStatus", modelStatusMethod)
		r.AddMethod("ModelManager", 3, "ModifyModelAccess", modifyModelAccessMethod)
		r.AddMethod("ModelManager", 3, "SetModelDefaults", setModelDefaultsMethod)
		r.AddMethod("ModelManager", 3, "UnsetModelDefaults", unsetModelDefaultsMethod)

		r.AddMethod("ModelManager", 4, "CreateModel", createModelMethod)
		r.AddMethod("ModelManager", 4, "DestroyModels", destroyModelsV4Method)
		r.AddMethod("ModelManager", 4, "DumpModels", dumpModelsMethod)
		r.AddMethod("ModelManager", 4, "DumpModelsDB", dumpModelsDBMethod)
		r.AddMethod("ModelManager", 4, "ListModelSummaries", listModelSummariesMethod)
		r.AddMethod("ModelManager", 4, "ListModels", listModelsMethod)
		r.AddMethod("ModelManager", 4, "ModelInfo", modelInfoMethod)
		r.AddMethod("ModelManager", 4, "ModelStatus", modelStatusMethod)
		r.AddMethod("ModelManager", 4, "ModifyModelAccess", modifyModelAccessMethod)
		r.AddMethod("ModelManager", 4, "SetModelDefaults", setModelDefaultsMethod)
		r.AddMethod("ModelManager", 4, "UnsetModelDefaults", unsetModelDefaultsMethod)

		r.AddMethod("ModelManager", 5, "ChangeModelCredential", changeModelCredentialMethod)
		r.AddMethod("ModelManager", 5, "CreateModel", createModelMethod)
		r.AddMethod("ModelManager", 5, "DestroyModels", destroyModelsV4Method)
		r.AddMethod("ModelManager", 5, "DumpModels", dumpModelsMethod)
		r.AddMethod("ModelManager", 5, "DumpModelsDB", dumpModelsDBMethod)
		r.AddMethod("ModelManager", 5, "ListModelSummaries", listModelSummariesMethod)
		r.AddMethod("ModelManager", 5, "ListModels", listModelsMethod)
		r.AddMethod("ModelManager", 5, "ModelInfo", modelInfoMethod)
		r.AddMethod("ModelManager", 5, "ModelStatus", modelStatusMethod)
		r.AddMethod("ModelManager", 5, "ModifyModelAccess", modifyModelAccessMethod)
		r.AddMethod("ModelManager", 5, "SetModelDefaults", setModelDefaultsMethod)
		r.AddMethod("ModelManager", 5, "UnsetModelDefaults", unsetModelDefaultsMethod)

		r.AddMethod("ModelManager", 6, "ChangeModelCredential", changeModelCredentialMethod)
		r.AddMethod("ModelManager", 6, "CreateModel", createModelMethod)
		r.AddMethod("ModelManager", 6, "DestroyModels", destroyModelsV4Method)
		r.AddMethod("ModelManager", 6, "DumpModels", dumpModelsMethod)
		r.AddMethod("ModelManager", 6, "DumpModelsDB", dumpModelsDBMethod)
		r.AddMethod("ModelManager", 6, "ListModelSummaries", listModelSummariesMethod)
		r.AddMethod("ModelManager", 6, "ListModels", listModelsMethod)
		r.AddMethod("ModelManager", 6, "ModelInfo", modelInfoMethod)
		r.AddMethod("ModelManager", 6, "ModelStatus", modelStatusMethod)
		r.AddMethod("ModelManager", 6, "ModifyModelAccess", modifyModelAccessMethod)
		r.AddMethod("ModelManager", 6, "SetModelDefaults", setModelDefaultsMethod)
		r.AddMethod("ModelManager", 6, "UnsetModelDefaults", unsetModelDefaultsMethod)
		r.AddMethod("ModelManager", 6, "ModelDefaultsForClouds", modelDefaultsForCloudsMethod)

		r.AddMethod("ModelManager", 7, "ChangeModelCredential", changeModelCredentialMethod)
		r.AddMethod("ModelManager", 7, "CreateModel", createModelMethod)
		r.AddMethod("ModelManager", 7, "DestroyModels", destroyModelsV4Method)
		r.AddMethod("ModelManager", 7, "DumpModels", dumpModelsMethod)
		r.AddMethod("ModelManager", 7, "DumpModelsDB", dumpModelsDBMethod)
		r.AddMethod("ModelManager", 7, "ListModelSummaries", listModelSummariesMethod)
		r.AddMethod("ModelManager", 7, "ListModels", listModelsMethod)
		r.AddMethod("ModelManager", 7, "ModelInfo", modelInfoMethod)
		r.AddMethod("ModelManager", 7, "ModelStatus", modelStatusMethod)
		r.AddMethod("ModelManager", 7, "ModifyModelAccess", modifyModelAccessMethod)
		r.AddMethod("ModelManager", 7, "SetModelDefaults", setModelDefaultsMethod)
		r.AddMethod("ModelManager", 7, "UnsetModelDefaults", unsetModelDefaultsMethod)
		r.AddMethod("ModelManager", 7, "ModelDefaultsForClouds", modelDefaultsForCloudsMethod)

		r.AddMethod("ModelManager", 8, "ChangeModelCredential", changeModelCredentialMethod)
		r.AddMethod("ModelManager", 8, "CreateModel", createModelMethod)
		r.AddMethod("ModelManager", 8, "DestroyModels", destroyModelsV4Method)
		r.AddMethod("ModelManager", 8, "DumpModels", dumpModelsMethod)
		r.AddMethod("ModelManager", 8, "DumpModelsDB", dumpModelsDBMethod)
		r.AddMethod("ModelManager", 8, "ListModelSummaries", listModelSummariesMethod)
		r.AddMethod("ModelManager", 8, "ListModels", listModelsMethod)
		r.AddMethod("ModelManager", 8, "ModelInfo", modelInfoMethod)
		r.AddMethod("ModelManager", 8, "ModelStatus", modelStatusMethod)
		r.AddMethod("ModelManager", 8, "ModifyModelAccess", modifyModelAccessMethod)
		r.AddMethod("ModelManager", 8, "SetModelDefaults", setModelDefaultsMethod)
		r.AddMethod("ModelManager", 8, "UnsetModelDefaults", unsetModelDefaultsMethod)
		r.AddMethod("ModelManager", 8, "ModelDefaultsForClouds", modelDefaultsForCloudsMethod)

		r.AddMethod("ModelManager", 9, "ChangeModelCredential", changeModelCredentialMethod)
		r.AddMethod("ModelManager", 9, "CreateModel", createModelMethod)
		r.AddMethod("ModelManager", 9, "DestroyModels", destroyModelsV4Method)
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

		return []int{2, 3, 4, 5, 6, 7, 8, 9}
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

func (r *controllerRoot) DestroyModels(ctx context.Context, args jujuparams.Entities) (jujuparams.ErrorResults, error) {
	// This is the default behviour for model manager V3 and below.
	destroyStorage := true
	models := make([]jujuparams.DestroyModelParams, len(args.Entities))
	for i, ent := range args.Entities {
		models[i] = jujuparams.DestroyModelParams{
			ModelTag:       ent.Tag,
			DestroyStorage: &destroyStorage,
		}
	}
	return r.DestroyModelsV4(ctx, jujuparams.DestroyModelsParams{models})
}

// ListModelSummaries returns summaries for all the models that that
// authenticated user has access to. The request parameter is ignored.
func (r *controllerRoot) ListModelSummaries(ctx context.Context, _ jujuparams.ModelSummariesRequest) (jujuparams.ModelSummaryResults, error) {
	const op = errors.Op("jujuapi.ListModelSummaries")

	var results []jujuparams.ModelSummaryResult
	err := r.jimm.ForEachUserModel(ctx, r.user, func(uma *dbmodel.UserModelAccess) error {
		ms := uma.ToJujuModelSummary()
		if r.controllerUUIDMasking {
			ms.ControllerUUID = r.params.ControllerUUID
		}
		result := jujuparams.ModelSummaryResult{
			Result: &ms,
		}
		results = append(results, result)
		return nil
	})
	if err != nil {
		return jujuparams.ModelSummaryResults{}, errors.E(op, err)
	}
	return jujuparams.ModelSummaryResults{
		Results: results,
	}, nil
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

	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()

	var mca jimm.ModelCreateArgs
	if err := mca.FromJujuModelCreateArgs(&args); err != nil {
		return jujuparams.ModelInfo{}, errors.E(op, err)
	}
	info, err := r.jimm.AddModel(ctx, r.user, &mca)
	if err == nil {
		servermon.ModelsCreatedCount.Inc()
		if r.controllerUUIDMasking {
			info.ControllerUUID = r.params.ControllerUUID
		}
		return *info, nil
	}
	servermon.ModelsCreatedFailCount.Inc()
	return jujuparams.ModelInfo{}, errors.E(op, err)
}

// DestroyModelsV4 implements the ModelManager facade's DestroyModels
// method used in version 4 onwards.
func (r *controllerRoot) DestroyModelsV4(ctx context.Context, args jujuparams.DestroyModelsParams) (jujuparams.ErrorResults, error) {
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
		results[i].Error = mapError(r.jimm.SetModelDefaults(ctx, r.user, cloudTag, config.CloudRegion, config.Config))
	}

	return jujuparams.ErrorResults{
		Results: results,
	}, nil
}

// UnsetModelDefaults removes the specified default model settings.
func (r *controllerRoot) UnsetModelDefaults(ctx context.Context, args jujuparams.UnsetModelDefaults) (jujuparams.ErrorResults, error) {
	const op = errors.Op("jujuapi.ModelDefaultsForClouds")

	results := make([]jujuparams.ErrorResult, len(args.Keys))
	for i, key := range args.Keys {
		cloudTag, err := names.ParseCloudTag(key.CloudTag)
		if err != nil {
			results[i].Error = mapError(err)
			continue
		}
		results[i].Error = mapError(r.jimm.UnsetModelDefaults(ctx, r.user, cloudTag, key.CloudRegion, key.Keys))
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
		defaults, err := r.jimm.ModelDefaultsForCloud(ctx, r.user, cloudTag)
		if err != nil {
			result.Results[i].Error = mapError(errors.E(op, err))
			continue
		}
		result.Results[i] = defaults
	}
	return result, nil
}

// TODO (ashipika) Implement ModelDefaults - need to determine which cloud this would refer to.
