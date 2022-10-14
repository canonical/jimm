// Copyright 2016 Canonical Ltd.

package jujuapi

import (
	"context"

	jujuparams "github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/life"
	"github.com/juju/names/v4"
	"github.com/juju/utils/parallel"
	"gopkg.in/errgo.v1"

	"github.com/CanonicalLtd/jimm/internal/apiconn"
	"github.com/CanonicalLtd/jimm/internal/conv"
	"github.com/CanonicalLtd/jimm/internal/jem"
	"github.com/CanonicalLtd/jimm/internal/jujuapi/rpc"
	"github.com/CanonicalLtd/jimm/internal/mongodoc"
	"github.com/CanonicalLtd/jimm/internal/servermon"
	"github.com/CanonicalLtd/jimm/params"
)

func init() {
	facadeInit["ModelManager"] = func(r *controllerRoot) []int {
		changeModelCredentialMethod := rpc.Method(r.ChangeModelCredential)
		createModelMethod := rpc.Method(r.CreateModel)
		destroyModelsMethod := rpc.Method(r.DestroyModels)
		destroyModelsV4Method := rpc.Method(r.DestroyModelsV4)
		dumpModelsMethod := rpc.Method(r.DumpModels)
		dumpModelsV3Method := rpc.Method(r.DumpModelsV3)
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
		r.AddMethod("ModelManager", 2, "DumpModels", dumpModelsMethod)
		r.AddMethod("ModelManager", 2, "DumpModelsDB", dumpModelsDBMethod)
		r.AddMethod("ModelManager", 2, "ListModels", listModelsMethod)
		r.AddMethod("ModelManager", 2, "ModelStatus", modelStatusMethod)
		r.AddMethod("ModelManager", 2, "ModifyModelAccess", modifyModelAccessMethod)
		r.AddMethod("ModelManager", 2, "SetModelDefaults", setModelDefaultsMethod)
		r.AddMethod("ModelManager", 2, "UnsetModelDefaults", unsetModelDefaultsMethod)

		r.AddMethod("ModelManager", 3, "CreateModel", createModelMethod)
		r.AddMethod("ModelManager", 3, "DestroyModels", destroyModelsMethod)
		r.AddMethod("ModelManager", 3, "DumpModels", dumpModelsV3Method)
		r.AddMethod("ModelManager", 3, "DumpModelsDB", dumpModelsDBMethod)
		r.AddMethod("ModelManager", 3, "ListModels", listModelsMethod)
		r.AddMethod("ModelManager", 3, "ModelInfo", modelInfoMethod)
		r.AddMethod("ModelManager", 3, "ModelStatus", modelStatusMethod)
		r.AddMethod("ModelManager", 3, "ModifyModelAccess", modifyModelAccessMethod)
		r.AddMethod("ModelManager", 3, "SetModelDefaults", setModelDefaultsMethod)
		r.AddMethod("ModelManager", 3, "UnsetModelDefaults", unsetModelDefaultsMethod)

		r.AddMethod("ModelManager", 4, "CreateModel", createModelMethod)
		r.AddMethod("ModelManager", 4, "DestroyModels", destroyModelsV4Method)
		r.AddMethod("ModelManager", 4, "DumpModels", dumpModelsV3Method)
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
		r.AddMethod("ModelManager", 5, "DumpModels", dumpModelsV3Method)
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
		r.AddMethod("ModelManager", 6, "DumpModels", dumpModelsV3Method)
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
		r.AddMethod("ModelManager", 7, "DumpModels", dumpModelsV3Method)
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
		r.AddMethod("ModelManager", 8, "DumpModels", dumpModelsV3Method)
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
		r.AddMethod("ModelManager", 9, "DumpModels", dumpModelsV3Method)
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

// DumpModels implements the DumpModels method of the modelmanager (v2)
// facade. The model dump is passed back as-is from the controller
// without any changes from JIMM.
func (r *controllerRoot) DumpModels(ctx context.Context, args jujuparams.Entities) jujuparams.MapResults {
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	results := make([]jujuparams.MapResult, len(args.Entities))
	for i, ent := range args.Entities {
		err := r.modelWithConnection(
			ctx,
			ent.Tag,
			jujuparams.ModelAdminAccess,
			func(ctx context.Context, conn *apiconn.Conn, model *mongodoc.Model) error {
				var err error
				results[i].Result, err = conn.DumpModel(ctx, model.UUID)
				return errgo.Mask(err, apiconn.IsAPIError)
			},
		)
		if errgo.Cause(err) == params.ErrNotFound {
			err = params.ErrUnauthorized
		}
		results[i].Error = mapError(err)
	}
	return jujuparams.MapResults{
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
	var results []jujuparams.ModelSummaryResult
	err := r.jem.ForEachModel(ctx, r.identity, jujuparams.ModelReadAccess, func(model *mongodoc.Model) error {
		access, err := r.modelAccess(ctx, model)
		if err != nil {
			return errgo.Mask(err)
		}
		controllerUUID := model.ControllerUUID
		if r.controllerUUIDMasking {
			controllerUUID = r.params.ControllerUUID
		}
		result := jujuparams.ModelSummaryResult{
			Result: &jujuparams.ModelSummary{
				Name:               string(model.Path.Name),
				Type:               model.Type,
				UUID:               model.UUID,
				ControllerUUID:     controllerUUID,
				ProviderType:       model.ProviderType,
				DefaultSeries:      model.DefaultSeries,
				CloudTag:           conv.ToCloudTag(model.Cloud).String(),
				CloudRegion:        model.CloudRegion,
				CloudCredentialTag: conv.ToCloudCredentialTag(model.Credential).String(),
				OwnerTag:           conv.ToUserTag(model.Path.User).String(),
				Life:               life.Value(model.Life()),
				Status:             modelStatus(model.Info),
				UserAccess:         access,
				// TODO currently user logins aren't communicated by the multiwatcher
				// so the UserLastConnection time is not known.
				UserLastConnection: nil,
				Counts: []jujuparams.ModelEntityCount{{
					Entity: jujuparams.Machines,
					Count:  int64(model.Counts[params.MachineCount].Current),
				}, {
					Entity: jujuparams.Cores,
					Count:  int64(model.Counts[params.CoreCount].Current),
				}, {
					Entity: jujuparams.Units,
					Count:  int64(model.Counts[params.UnitCount].Current),
				}},
				// TODO currently we don't store any migration information about models.
				Migration: nil,
				// TODO currently we don't store any SLA information.
				SLA:          nil,
				AgentVersion: modelVersion(ctx, model.Info),
			},
		}
		results = append(results, result)
		return nil
	})
	if err != nil {
		return jujuparams.ModelSummaryResults{}, errgo.Mask(err)
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
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	results := make([]jujuparams.ModelInfoResult, len(args.Entities))
	run := parallel.NewRun(maxRequestConcurrency)
	for i, arg := range args.Entities {
		mt, err := names.ParseModelTag(arg.Tag)
		if err != nil {
			results[i].Error = mapError(errgo.WithCausef(err, params.ErrBadRequest, ""))
			continue
		}
		results[i].Result = &jujuparams.ModelInfo{
			UUID: mt.Id(),
		}
		i := i
		run.Do(func() error {
			err := r.jem.GetModelInfo(ctx, r.identity, results[i].Result, len(results) == 1)
			if errgo.Cause(err) == params.ErrNotFound {
				// Map not-found errors to unauthorized, this is what juju
				// does.
				err = params.ErrUnauthorized
			}
			results[i].Error = mapError(err)
			if r.controllerUUIDMasking {
				results[i].Result.ControllerUUID = r.params.ControllerUUID
			}
			if results[i].Error != nil {
				results[i].Result = nil
			}
			return nil
		})
	}
	run.Wait()
	return jujuparams.ModelInfoResults{
		Results: results,
	}, nil
}

// CreateModel implements the ModelManager facade's CreateModel method.
func (r *controllerRoot) CreateModel(ctx context.Context, args jujuparams.ModelCreateArgs) (info jujuparams.ModelInfo, err error) {
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	err = errgo.Mask(r.createModel(ctx, args, &info),
		errgo.Is(conv.ErrLocalUser),
		errgo.Is(params.ErrUnauthorized),
		errgo.Is(params.ErrNotFound),
		errgo.Is(params.ErrBadRequest),
	)
	if err == nil {
		servermon.ModelsCreatedCount.Inc()
	} else {
		servermon.ModelsCreatedFailCount.Inc()
	}
	return
}

func (r *controllerRoot) createModel(ctx context.Context, args jujuparams.ModelCreateArgs, info *jujuparams.ModelInfo) error {
	owner, err := conv.ParseUserTag(args.OwnerTag)
	if err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrBadRequest), errgo.Is(conv.ErrLocalUser))
	}
	if args.CloudTag == "" {
		return errgo.New("no cloud specified for model; please specify one")
	}
	cloudTag, err := names.ParseCloudTag(args.CloudTag)
	if err != nil {
		return errgo.WithCausef(err, params.ErrBadRequest, "invalid cloud tag")
	}
	cloud := params.Cloud(cloudTag.Id())
	var credPath mongodoc.CredentialPath
	if args.CloudCredentialTag != "" {
		tag, err := names.ParseCloudCredentialTag(args.CloudCredentialTag)
		if err != nil {
			return errgo.WithCausef(err, params.ErrBadRequest, "invalid cloud credential tag")
		}
		credPath, err = conv.FromCloudCredentialTag(tag)
		if err != nil {
			return errgo.Mask(err, errgo.Is(conv.ErrLocalUser))
		}
	}
	err = r.jem.CreateModel(ctx, r.identity, jem.CreateModelParams{
		Path:       params.EntityPath{User: owner, Name: params.Name(args.Name)},
		Credential: credPath,
		Cloud:      cloud,
		Region:     args.CloudRegion,
		Attributes: args.Config,
	}, info)
	if err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrBadRequest), errgo.Is(params.ErrNotFound), errgo.Is(params.ErrUnauthorized))
	}
	if r.controllerUUIDMasking {
		info.ControllerUUID = r.params.ControllerUUID
	}
	return nil
}

// DestroyModelsV4 implements the ModelManager facade's DestroyModels
// method used in version 4 onwards.
func (r *controllerRoot) DestroyModelsV4(ctx context.Context, args jujuparams.DestroyModelsParams) (jujuparams.ErrorResults, error) {
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	results := make([]jujuparams.ErrorResult, len(args.Models))

	for i, model := range args.Models {
		mt, err := names.ParseModelTag(model.ModelTag)
		if err != nil {
			results[i].Error = mapError(errgo.WithCausef(err, params.ErrBadRequest, ""))
			continue
		}

		if err := r.jem.DestroyModel(ctx, r.identity, &mongodoc.Model{UUID: mt.Id()}, model.DestroyStorage, model.Force, model.MaxWait); err != nil {
			if errgo.Cause(err) != params.ErrNotFound {
				// It isn't an error to try and destroy an already
				// destroyed model.
				results[i].Error = mapError(err)
			}
		}
	}

	return jujuparams.ErrorResults{
		Results: results,
	}, nil
}

// ModifyModelAccess implements the ModelManager facade's ModifyModelAccess method.
func (r *controllerRoot) ModifyModelAccess(ctx context.Context, args jujuparams.ModifyModelAccessRequest) (jujuparams.ErrorResults, error) {
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	results := make([]jujuparams.ErrorResult, len(args.Changes))
	for i, change := range args.Changes {
		mt, err := names.ParseModelTag(change.ModelTag)
		if err != nil {
			results[i].Error = mapError(errgo.WithCausef(err, params.ErrBadRequest, ""))
			continue
		}
		user, err := conv.ParseUserTag(change.UserTag)
		if err != nil {
			results[i].Error = mapError(errgo.WithCausef(err, params.ErrBadRequest, ""))
			continue
		}
		switch change.Action {
		case jujuparams.GrantModelAccess:
			err = r.jem.GrantModel(ctx, r.identity, &mongodoc.Model{UUID: mt.Id()}, user, change.Access)
		case jujuparams.RevokeModelAccess:
			err = r.jem.RevokeModel(ctx, r.identity, &mongodoc.Model{UUID: mt.Id()}, user, change.Access)
		default:
			err = errgo.WithCausef(err, params.ErrBadRequest, "invalid action %q", change.Action)
		}
		if errgo.Cause(err) == params.ErrNotFound {
			err = params.ErrUnauthorized
		}
		results[i].Error = mapError(err)
	}
	return jujuparams.ErrorResults{
		Results: results,
	}, nil
}

// DumpModelsV3 implements the ModelManager (version 3 onwards) facade's
// DumpModels API. The model dump is passed back as-is from the
// controller without any changes from JIMM.
func (r *controllerRoot) DumpModelsV3(ctx context.Context, args jujuparams.DumpModelRequest) jujuparams.StringResults {
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	results := make([]jujuparams.StringResult, len(args.Entities))
	for i, ent := range args.Entities {
		err := r.modelWithConnection(
			ctx,
			ent.Tag,
			jujuparams.ModelAdminAccess,
			func(ctx context.Context, conn *apiconn.Conn, model *mongodoc.Model) error {
				var err error
				results[i].Result, err = conn.DumpModelV3(ctx, model.UUID, args.Simplified)
				return errgo.Mask(err, apiconn.IsAPIError)
			},
		)
		if errgo.Cause(err) == params.ErrNotFound {
			err = params.ErrUnauthorized
		}
		results[i].Error = mapError(err)
	}
	return jujuparams.StringResults{
		Results: results,
	}
}

// DumpModelsDB implements the modelmanager facades DumpModelsDB API. The
// model dump is passed back as-is from the controller without any
// changes from JIMM.
func (r *controllerRoot) DumpModelsDB(ctx context.Context, args jujuparams.Entities) jujuparams.MapResults {
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	results := make([]jujuparams.MapResult, len(args.Entities))
	for i, ent := range args.Entities {
		err := r.modelWithConnection(
			ctx,
			ent.Tag,
			jujuparams.ModelAdminAccess,
			func(ctx context.Context, conn *apiconn.Conn, model *mongodoc.Model) error {
				var err error
				results[i].Result, err = conn.DumpModelDB(ctx, model.UUID)
				return errgo.Mask(err, apiconn.IsAPIError)
			},
		)
		if errgo.Cause(err) == params.ErrNotFound {
			err = params.ErrUnauthorized
		}
		results[i].Error = mapError(err)

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
	mt, err := names.ParseModelTag(arg.ModelTag)
	if err != nil {
		return errgo.WithCausef(err, params.ErrBadRequest, "")
	}
	model := mongodoc.Model{UUID: mt.Id()}
	if err := r.jem.GetModel(ctx, r.identity, jujuparams.ModelAdminAccess, &model); err != nil {
		return errgo.Mask(
			err,
			errgo.Is(params.ErrBadRequest),
			errgo.Is(params.ErrUnauthorized),
			errgo.Is(params.ErrNotFound),
		)
	}
	conn, err := r.jem.OpenAPI(ctx, model.Controller)
	if err != nil {
		return errgo.Mask(err)
	}
	credTag, err := names.ParseCloudCredentialTag(arg.CloudCredentialTag)
	if err != nil {
		return errgo.WithCausef(err, params.ErrBadRequest, "invalid credential tag")
	}
	credPath, err := conv.FromCloudCredentialTag(credTag)
	if err != nil {
		return errgo.Mask(err, errgo.Is(conv.ErrLocalUser))
	}
	cred := mongodoc.Credential{
		Path: credPath,
	}
	if err := r.jem.GetCredential(ctx, r.identity, &cred); err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrNotFound), errgo.Is(params.ErrUnauthorized))
	}
	if err := r.jem.UpdateModelCredential(ctx, conn, &model, &cred); err != nil {
		return errgo.Mask(err)
	}
	return nil
}

// ValidateModelUpgrades validates if a model is allowed to perform an upgrade.
// Examples of why you would want to block a model upgrade, would be situations
// like upgrade-series. If performing an upgrade-series we don't know the
// current status of the machine, so performing an upgrade-model can lead to
// bad unintended errors down the line.
func (r *controllerRoot) ValidateModelUpgrades(ctx context.Context, args jujuparams.ValidateModelUpgradeParams) (jujuparams.ErrorResults, error) {
	results := make([]jujuparams.ErrorResult, len(args.Models))
	for i, arg := range args.Models {
		modelTag, err := names.ParseModelTag(arg.ModelTag)
		if err != nil {
			results[i].Error = mapError(errgo.WithCausef(err, params.ErrBadRequest, ""))
			continue
		}
		results[i].Error = mapError(r.jem.ValidateModelUpgrade(ctx, r.identity, modelTag.Id(), args.Force))
	}
	return jujuparams.ErrorResults{
		Results: results,
	}, nil
}

// SetModelDefaults writes new values for the specified default model settings.
func (r *controllerRoot) SetModelDefaults(ctx context.Context, args jujuparams.SetModelDefaults) (jujuparams.ErrorResults, error) {
	if r.identity.Id() == "" {
		return jujuparams.ErrorResults{}, errgo.WithCausef(nil, params.ErrUnauthorized, "")
	}
	results := make([]jujuparams.ErrorResult, len(args.Config))
	for i, config := range args.Config {
		cloudTag, err := names.ParseCloudTag(config.CloudTag)
		if err != nil {
			results[i].Error = mapError(err)
			continue
		}
		cloud := string(conv.FromCloudTag(cloudTag))
		results[i].Error = mapError(r.jem.SetModelDefaults(ctx, r.identity, cloud, config.CloudRegion, config.Config))
	}

	return jujuparams.ErrorResults{
		Results: results,
	}, nil
}

// UnsetModelDefaults removes the specified default model settings.
func (r *controllerRoot) UnsetModelDefaults(ctx context.Context, args jujuparams.UnsetModelDefaults) (jujuparams.ErrorResults, error) {
	if r.identity.Id() == "" {
		return jujuparams.ErrorResults{}, errgo.WithCausef(nil, params.ErrUnauthorized, "")
	}
	results := make([]jujuparams.ErrorResult, len(args.Keys))
	for i, key := range args.Keys {
		cloudTag, err := names.ParseCloudTag(key.CloudTag)
		if err != nil {
			results[i].Error = mapError(err)
			continue
		}
		cloud := string(conv.FromCloudTag(cloudTag))
		results[i].Error = mapError(r.jem.UnsetModelDefaults(ctx, r.identity, cloud, key.CloudRegion, key.Keys))
	}

	return jujuparams.ErrorResults{
		Results: results,
	}, nil
}

// ModelDefaultsForClouds returns the default config values for the specified
// clouds.
func (r *controllerRoot) ModelDefaultsForClouds(ctx context.Context, args jujuparams.Entities) (jujuparams.ModelDefaultsResults, error) {
	if r.identity.Id() == "" {
		return jujuparams.ModelDefaultsResults{}, errgo.WithCausef(nil, params.ErrUnauthorized, "")
	}
	result := jujuparams.ModelDefaultsResults{}
	result.Results = make([]jujuparams.ModelDefaultsResult, len(args.Entities))
	for i, entity := range args.Entities {
		cloudTag, err := names.ParseCloudTag(entity.Tag)
		if err != nil {
			result.Results[i].Error = mapError(err)
			continue
		}
		cloud := conv.FromCloudTag(cloudTag)
		defaults, err := r.jem.ModelDefaultsForCloud(ctx, r.identity, cloud)
		if err != nil {
			result.Results[i].Error = mapError(err)
			continue
		}
		result.Results[i] = defaults
	}
	return result, nil
}

// TODO (ashipika) Implement ModelDefaults - need to determine which cloud this would refer to.
