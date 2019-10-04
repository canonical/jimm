// Copyright 2016 Canonical Ltd.

package jujuapi

import (
	"context"
	"time"

	"github.com/juju/juju/apiserver/common"
	jujuparams "github.com/juju/juju/apiserver/params"
	"github.com/juju/utils/parallel"
	"gopkg.in/errgo.v1"
	"gopkg.in/juju/names.v3"

	"github.com/CanonicalLtd/jimm/internal/auth"
	"github.com/CanonicalLtd/jimm/internal/ctxutil"
	"github.com/CanonicalLtd/jimm/internal/jem"
	"github.com/CanonicalLtd/jimm/internal/mongodoc"
	"github.com/CanonicalLtd/jimm/internal/servermon"
	"github.com/CanonicalLtd/jimm/params"
)

// ModelManagerV2 returns an implementation of the ModelManager facade
// (version 2).
func (r *controllerRoot) ModelManagerV2(id string) (modelManagerV2, error) {
	mm, err := r.ModelManagerV3(id)
	return modelManagerV2{mm}, err
}

type modelManagerV2 struct {
	modelManagerV3
}

// DumpModels implements the DumpModels method of the modelmanager (v2)
// facade. Actually dumping the model is not yet supported, so it always
// returns an error.
func (m modelManagerV2) DumpModels(ctx context.Context, args jujuparams.Entities) jujuparams.MapResults {
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	ctx = ctxutil.Join(ctx, m.root.authContext)
	results := make([]jujuparams.MapResult, len(args.Entities))
	for i, ent := range args.Entities {
		_, err := m.dumpModel(ctx, ent, false)
		results[i].Error = mapError(err)
	}
	return jujuparams.MapResults{
		Results: results,
	}
}

// ModelManagerV3 returns an implementation of the ModelManager facade
// (version 3).
func (r *controllerRoot) ModelManagerV3(id string) (modelManagerV3, error) {
	mm, err := r.ModelManagerAPI(id)
	return modelManagerV3{mm}, err
}

type modelManagerV3 struct {
	modelManagerAPI
}

func (m modelManagerV3) DestroyModels(ctx context.Context, args jujuparams.Entities) (jujuparams.ErrorResults, error) {
	// This is the default behviour for model manager V3 and below.
	destroyStorage := true
	models := make([]jujuparams.DestroyModelParams, len(args.Entities))
	for i, ent := range args.Entities {
		models[i] = jujuparams.DestroyModelParams{
			ModelTag:       ent.Tag,
			DestroyStorage: &destroyStorage,
		}
	}
	return m.modelManagerAPI.DestroyModels(ctx, jujuparams.DestroyModelsParams{models})
}

// ModelManagerAPI returns an implementation of the latest ModelManager
// facade.
func (r *controllerRoot) ModelManagerAPI(id string) (modelManagerAPI, error) {
	if id != "" {
		// Safeguard id for possible future use.
		return modelManagerAPI{}, common.ErrBadId
	}
	return modelManagerAPI{r}, nil
}

// modelManagerAPI implements the latest ModelManager facade.
type modelManagerAPI struct {
	root *controllerRoot
}

// ListModelSummaries returns summaries for all the models that that
// authenticated user has access to. The request parameter is ignored.
func (m modelManagerAPI) ListModelSummaries(ctx context.Context, _ jujuparams.ModelSummariesRequest) (jujuparams.ModelSummaryResults, error) {
	ctx = ctxutil.Join(ctx, m.root.authContext)
	var results []jujuparams.ModelSummaryResult
	err := m.root.doModels(ctx, func(ctx context.Context, model *mongodoc.Model) error {
		if model.ProviderType == "" {
			var err error
			model.ProviderType, err = m.root.jem.DB.ProviderType(ctx, model.Cloud)
			if err != nil {
				results = append(results, jujuparams.ModelSummaryResult{
					Error: mapError(errgo.Notef(err, "cannot get cloud %q", model.Cloud)),
				})
				return nil
			}
		}
		// If we get this far the user must have at least read access.
		access := jujuparams.ModelReadAccess
		switch {
		case params.User(auth.Username(ctx)) == model.Path.User:
			access = jujuparams.ModelAdminAccess
		case auth.CheckACL(ctx, model.ACL.Admin) == nil:
			access = jujuparams.ModelAdminAccess
		case auth.CheckACL(ctx, model.ACL.Write) == nil:
			access = jujuparams.ModelWriteAccess
		}
		machines, err := m.root.jem.DB.MachinesForModel(ctx, model.UUID)
		if err != nil {
			results = append(results, jujuparams.ModelSummaryResult{
				Error: mapError(errgo.Notef(err, "cannot get machines for model %q", model.UUID)),
			})
			return nil
		}
		machineCount := int64(len(machines))
		var coreCount int64
		for _, machine := range machines {
			if machine.Info != nil &&
				machine.Info.HardwareCharacteristics != nil &&
				machine.Info.HardwareCharacteristics.CpuCores != nil {
				coreCount += int64(*machine.Info.HardwareCharacteristics.CpuCores)
			}
		}
		results = append(results, jujuparams.ModelSummaryResult{
			Result: &jujuparams.ModelSummary{
				Name:               string(model.Path.Name),
				Type:               model.Type,
				UUID:               model.UUID,
				ControllerUUID:     m.root.params.ControllerUUID,
				ProviderType:       model.ProviderType,
				DefaultSeries:      model.DefaultSeries,
				CloudTag:           jem.CloudTag(model.Cloud).String(),
				CloudRegion:        model.CloudRegion,
				CloudCredentialTag: jem.CloudCredentialTag(model.Credential).String(),
				OwnerTag:           jem.UserTag(model.Path.User).String(),
				Life:               jujuparams.Life(model.Life()),
				Status:             modelStatus(model.Info),
				UserAccess:         access,
				// TODO currently user logins aren't communicated by the multiwatcher
				// so the UserLastConnection time is not known.
				UserLastConnection: nil,
				Counts: []jujuparams.ModelEntityCount{{
					Entity: jujuparams.Machines,
					Count:  machineCount,
				}, {
					Entity: jujuparams.Cores,
					Count:  coreCount,
				}},
				// TODO currently we don't store any migration information about models.
				Migration: nil,
				// TODO currently we don't store any SLA information.
				SLA:          nil,
				AgentVersion: modelVersion(ctx, model.Info),
			},
		})
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
func (m modelManagerAPI) ListModels(ctx context.Context, _ jujuparams.Entity) (jujuparams.UserModelList, error) {
	ctx = ctxutil.Join(ctx, m.root.authContext)
	return m.root.allModels(ctx)
}

// ModelInfo implements the ModelManager facade's ModelInfo method.
func (m modelManagerAPI) ModelInfo(ctx context.Context, args jujuparams.Entities) (jujuparams.ModelInfoResults, error) {
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	ctx = ctxutil.Join(ctx, m.root.authContext)
	results := make([]jujuparams.ModelInfoResult, len(args.Entities))
	run := parallel.NewRun(maxRequestConcurrency)
	for i, arg := range args.Entities {
		i, arg := i, arg
		run.Do(func() error {
			mi, err := m.root.modelInfo(ctx, arg, len(args.Entities) != 1)
			if err != nil {
				results[i].Error = mapError(err)
			} else {
				results[i].Result = mi
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
func (m modelManagerAPI) CreateModel(ctx context.Context, args jujuparams.ModelCreateArgs) (jujuparams.ModelInfo, error) {
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	ctx = ctxutil.Join(ctx, m.root.authContext)
	mi, err := m.createModel(ctx, args)
	if err == nil {
		servermon.ModelsCreatedCount.Inc()
	} else {
		servermon.ModelsCreatedFailCount.Inc()
	}
	if err != nil {
		return jujuparams.ModelInfo{}, errgo.Mask(err,
			errgo.Is(params.ErrUnauthorized),
			errgo.Is(params.ErrNotFound),
			errgo.Is(params.ErrBadRequest),
		)
	}
	return *mi, nil
}

func (m modelManagerAPI) createModel(ctx context.Context, args jujuparams.ModelCreateArgs) (*jujuparams.ModelInfo, error) {
	ownerTag, err := names.ParseUserTag(args.OwnerTag)
	if err != nil {
		return nil, errgo.WithCausef(err, params.ErrBadRequest, "invalid owner tag")
	}
	owner, err := user(ownerTag)
	if err != nil {
		return nil, errgo.Mask(err, errgo.Is(params.ErrBadRequest))
	}
	if args.CloudTag == "" {
		return nil, errgo.New("no cloud specified for model; please specify one")
	}
	cloudTag, err := names.ParseCloudTag(args.CloudTag)
	if err != nil {
		return nil, errgo.WithCausef(err, params.ErrBadRequest, "invalid cloud tag")
	}
	cloud := params.Cloud(cloudTag.Id())
	var credPath params.CredentialPath
	if args.CloudCredentialTag != "" {
		tag, err := names.ParseCloudCredentialTag(args.CloudCredentialTag)
		if err != nil {
			return nil, errgo.WithCausef(err, params.ErrBadRequest, "invalid cloud credential tag")
		}
		credPath = params.CredentialPath{
			Cloud: params.Cloud(tag.Cloud().Id()),
			EntityPath: params.EntityPath{
				User: owner,
				Name: params.Name(tag.Name()),
			},
		}
	}
	model, err := m.root.jem.CreateModel(ctx, jem.CreateModelParams{
		Path:       params.EntityPath{User: owner, Name: params.Name(args.Name)},
		Credential: credPath,
		Cloud:      cloud,
		Region:     args.CloudRegion,
		Attributes: args.Config,
	})
	if err != nil {
		return nil, errgo.Mask(err, errgo.Is(params.ErrBadRequest), errgo.Is(params.ErrNotFound), errgo.Is(params.ErrUnauthorized))
	}
	info, err := m.root.modelDocToModelInfo(ctx, model)
	if err != nil {
		return nil, errgo.Mask(err)
	}

	return info, nil
}

// DestroyModels implements the ModelManager facade's DestroyModels method.
func (m modelManagerAPI) DestroyModels(ctx context.Context, args jujuparams.DestroyModelsParams) (jujuparams.ErrorResults, error) {
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	ctx = ctxutil.Join(ctx, m.root.authContext)
	results := make([]jujuparams.ErrorResult, len(args.Models))

	for i, model := range args.Models {
		if err := m.destroyModel(ctx, model); err != nil {
			results[i].Error = mapError(err)
		}
	}

	return jujuparams.ErrorResults{
		Results: results,
	}, nil
}

// destroyModel destroys the specified model.
func (m modelManagerAPI) destroyModel(ctx context.Context, arg jujuparams.DestroyModelParams) error {
	model, err := getModel(ctx, m.root.jem, arg.ModelTag, auth.CheckIsAdmin)
	if err != nil {
		if errgo.Cause(err) == params.ErrNotFound {
			// Juju doesn't treat removing a model that isn't there as an error, and neither should we.
			return nil
		}
		return errgo.Mask(err, errgo.Is(params.ErrBadRequest), errgo.Is(params.ErrUnauthorized))
	}
	conn, err := m.root.jem.OpenAPI(ctx, model.Controller)
	if err != nil {
		return errgo.Mask(err)
	}
	defer conn.Close()
	if err := m.root.jem.DestroyModel(ctx, conn, model, arg.DestroyStorage, arg.Force, arg.MaxWait); err != nil {
		return errgo.Mask(err, jujuparams.IsCodeHasPersistentStorage)
	}
	age := float64(time.Now().Sub(model.CreationTime)) / float64(time.Hour)
	servermon.ModelLifetime.Observe(age)
	servermon.ModelsDestroyedCount.Inc()
	return nil
}

// ModifyModelAccess implements the ModelManager facade's ModifyModelAccess method.
func (m modelManagerAPI) ModifyModelAccess(ctx context.Context, args jujuparams.ModifyModelAccessRequest) (jujuparams.ErrorResults, error) {
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	ctx = ctxutil.Join(ctx, m.root.authContext)
	results := make([]jujuparams.ErrorResult, len(args.Changes))
	for i, change := range args.Changes {
		err := m.modifyModelAccess(ctx, change)
		if err != nil {
			results[i].Error = mapError(err)
		}
	}
	return jujuparams.ErrorResults{
		Results: results,
	}, nil
}

func (m modelManagerAPI) modifyModelAccess(ctx context.Context, change jujuparams.ModifyModelAccess) error {
	model, err := getModel(ctx, m.root.jem, change.ModelTag, auth.CheckIsAdmin)
	if err != nil {
		if errgo.Cause(err) == params.ErrNotFound {
			err = params.ErrUnauthorized
		}
		return errgo.Mask(err, errgo.Is(params.ErrBadRequest), errgo.Is(params.ErrUnauthorized))
	}
	userTag, err := names.ParseUserTag(change.UserTag)
	if err != nil {
		return errgo.WithCausef(err, params.ErrBadRequest, "invalid user tag")
	}
	user, err := user(userTag)
	if err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrBadRequest))
	}
	conn, err := m.root.jem.OpenAPI(ctx, model.Controller)
	if err != nil {
		return errgo.Mask(err)
	}
	defer conn.Close()
	switch change.Action {
	case jujuparams.GrantModelAccess:
		err = m.root.jem.GrantModel(ctx, conn, model, user, string(change.Access))
	case jujuparams.RevokeModelAccess:
		err = m.root.jem.RevokeModel(ctx, conn, model, user, string(change.Access))
	default:
		return errgo.WithCausef(err, params.ErrBadRequest, "invalid action %q", change.Action)
	}
	if err != nil {
		return errgo.Mask(err)
	}
	return nil
}

// DumpModels implements the modelmanager facades DumpModels API.
// Actually dumping the model is not yet supported, so it always returns
// an error.
func (m modelManagerAPI) DumpModels(ctx context.Context, args jujuparams.DumpModelRequest) jujuparams.StringResults {
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	ctx = ctxutil.Join(ctx, m.root.authContext)
	results := make([]jujuparams.StringResult, len(args.Entities))
	for i, ent := range args.Entities {
		res, err := m.dumpModel(ctx, ent, args.Simplified)
		if err != nil {
			results[i].Error = mapError(err)
			continue
		}
		results[i].Result = res
	}
	return jujuparams.StringResults{
		Results: results,
	}
}

func (m modelManagerAPI) dumpModel(ctx context.Context, ent jujuparams.Entity, simplified bool) (string, error) {
	_, err := getModel(ctx, m.root.jem, ent.Tag, auth.CheckIsAdmin)
	if err != nil {
		if errgo.Cause(err) == params.ErrNotFound {
			err = params.ErrUnauthorized
		}
		return "", errgo.Mask(err, errgo.Is(params.ErrBadRequest), errgo.Is(params.ErrUnauthorized))
	}
	return "", errgo.WithCausef(nil, errNotImplemented, "DumpModel is not implemented for JAAS models")
}

// DumpModelsDB implements the modelmanager facades DumpModelsDB API.
// Actually dumping the model is not yet supported, so it always returns
// an error.
func (m modelManagerAPI) DumpModelsDB(ctx context.Context, args jujuparams.Entities) jujuparams.MapResults {
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	ctx = ctxutil.Join(ctx, m.root.authContext)
	results := make([]jujuparams.MapResult, len(args.Entities))
	for i, ent := range args.Entities {
		res, err := m.dumpModelDB(ctx, ent)
		if err != nil {
			results[i].Error = mapError(err)
			continue
		}
		results[i].Result = res
	}
	return jujuparams.MapResults{
		Results: results,
	}
}

func (m modelManagerAPI) dumpModelDB(ctx context.Context, ent jujuparams.Entity) (map[string]interface{}, error) {
	_, err := getModel(ctx, m.root.jem, ent.Tag, auth.CheckIsAdmin)
	if err != nil {
		if errgo.Cause(err) == params.ErrNotFound {
			err = params.ErrUnauthorized
		}
		return nil, errgo.Mask(err, errgo.Is(params.ErrBadRequest), errgo.Is(params.ErrUnauthorized))
	}
	return nil, errgo.WithCausef(nil, errNotImplemented, "DumpModelDB is not implemented for JAAS models")
}

// ModelStatus implements the ModelManager facade's ModelStatus method.
func (m modelManagerAPI) ModelStatus(ctx context.Context, req jujuparams.Entities) (jujuparams.ModelStatusResults, error) {
	return controller{m.root}.ModelStatus(ctx, req)
}

// ChangeModelCredential implements the ModelManager (v5) facade's
// ChangeModelCredential method.
func (m modelManagerAPI) ChangeModelCredential(ctx context.Context, args jujuparams.ChangeModelCredentialsParams) (jujuparams.ErrorResults, error) {
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	ctx = ctxutil.Join(ctx, m.root.authContext)
	results := make([]jujuparams.ErrorResult, len(args.Models))
	for i, arg := range args.Models {
		results[i].Error = mapError(m.changeModelCredential(ctx, arg))
	}
	return jujuparams.ErrorResults{
		Results: results,
	}, nil
}

func (m modelManagerAPI) changeModelCredential(ctx context.Context, arg jujuparams.ChangeModelCredentialParams) error {
	model, err := getModel(ctx, m.root.jem, arg.ModelTag, auth.CheckIsAdmin)
	if err != nil {
		return errgo.Mask(
			err,
			errgo.Is(params.ErrBadRequest),
			errgo.Is(params.ErrUnauthorized),
			errgo.Is(params.ErrNotFound),
		)
	}
	conn, err := m.root.jem.OpenAPI(ctx, model.Controller)
	if err != nil {
		return errgo.Mask(err)
	}
	credTag, err := names.ParseCloudCredentialTag(arg.CloudCredentialTag)
	if err != nil {
		return errgo.WithCausef(err, params.ErrBadRequest, "invalid credential tag")
	}
	credUser, err := user(credTag.Owner())
	if err != nil {
		return errgo.WithCausef(err, params.ErrBadRequest, "")
	}
	credPath := params.CredentialPath{
		Cloud: params.Cloud(credTag.Cloud().Id()),
		EntityPath: params.EntityPath{
			User: credUser,
			Name: params.Name(credTag.Name()),
		},
	}
	cred, err := m.root.jem.Credential(ctx, credPath)
	if err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrNotFound), errgo.Is(params.ErrUnauthorized))
	}
	if err := m.root.jem.UpdateModelCredential(ctx, conn, model, cred); err != nil {
		return errgo.Mask(err)
	}
	return nil
}
