// Copyright 2025 Canonical.

package juju

import (
	"context"
	"encoding/json"
	stderrors "errors"
	"fmt"
	"sync"

	"github.com/itchyny/gojq"
	jujucmd "github.com/juju/juju/cmd/cmd"
	"github.com/juju/juju/cmd/juju/status"
	"github.com/juju/juju/cmd/juju/storage"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v6"
	"github.com/juju/zaputil/zapctx"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"

	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/pkg/api/params"
)

const maxConcurrentModelQueries = 10

// QueryModels queries every specified model in modelUUIDs.
//
// The jqQuery must be a valid jq query and can return every result, even iterative listings.
// If a result is erroneous, for example, bad data type parsing, the resulting struct field
// Errors will contain a map from model UUID -> []error. Otherwise, the Results field
// will contain model UUID -> []Jq result.
//
//nolint:gocognit
func (j *JujuManager) QueryModelsJq(ctx context.Context, modelUUIDs []string, jqQuery string) (params.CrossModelQueryResponse, error) {
	results := params.CrossModelQueryResponse{
		Results: make(map[string][]any),
		Errors:  make(map[string][]string),
	}

	query, err := gojq.Parse(jqQuery)
	if err != nil {
		return results, fmt.Errorf("failed to parse jq query: %w", err)
	}

	models, err := j.Database.GetModelsByUUID(ctx, modelUUIDs)
	if err != nil {
		return results, errors.New("failed to get models for user")
	}

	g, _ := errgroup.WithContext(ctx)
	g.SetLimit(maxConcurrentModelQueries)
	m := sync.Mutex{}
	addItem := func(modelUUID string, result any, err error) {
		m.Lock()
		defer m.Unlock()
		if err != nil {
			results.Errors[modelUUID] = append(results.Errors[modelUUID], err.Error())
			return
		}
		results.Results[modelUUID] = append(results.Results[modelUUID], result)
	}

	for _, model := range models {
		g.Go(func() error {
			var err error
			modelUUID := model.UUID.String
			// Set up a formatterParamsRetriever to handle the heavy lifting
			// of each facade call and type conversion.
			retriever := newFormatterParamsRetriever(j)
			params, err := retriever.GetParams(ctx, model)
			if err != nil {
				zapctx.Error(ctx, "failed to get status formatter params", zap.String("model-uuid", modelUUID))
				addItem(modelUUID, nil, err)
				return nil
			}

			// We use very specific formatting parameters to ensure like-for-like output
			// with the default juju client installation performing a "status --format json".
			formatter := status.NewStatusFormatter(*params)

			formattedStatus, err := formatter.Format()
			if err != nil {
				zapctx.Error(ctx, "failed to format status", zap.String("model-uuid", modelUUID))
				addItem(modelUUID, nil, err)
				return nil
			}
			// We could use output.NewFormatter() from 3.0+ juju/juju, but ultimately
			// we just want some JSON output, regardless of user formatting. As such json.Marshal
			// *should* be OK.
			fb, err := json.Marshal(formattedStatus)
			if err != nil {
				zapctx.Error(ctx, "failed to marshal formatted status", zap.String("model-uuid", modelUUID))
				addItem(modelUUID, nil, err)
				return nil
			}
			tempMap := make(map[string]any)
			if err := json.Unmarshal(fb, &tempMap); err != nil {
				return err
			}

			queryCtx, cancel := context.WithTimeout(ctx, j.crossModelQueryTimeout)
			defer cancel()
			queryIter := query.RunWithContext(queryCtx, tempMap)

			for {
				v, ok := queryIter.Next()
				if !ok {
					break
				}

				// Jq errors can range from one failure in an iterative query to an entirely broken
				// query. As such, we simply append all to the errors field and continue to collect
				// both erreoneous and valid query results.
				if err, ok := v.(error); ok {
					if stderrors.Is(err, context.DeadlineExceeded) {
						return fmt.Errorf("jq query timed out after %.2f seconds: %w", j.crossModelQueryTimeout.Seconds(), err)
					}
					addItem(modelUUID, nil, fmt.Errorf("jq error: %w", err))
					continue
				}

				addItem(modelUUID, v, nil)
			}
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return results, err
	}
	return results, nil
}

// formatterParamsRetriever is a self-contained block of
// parameter retrieval for Juju's status.NewStatusFormatter.
//
// It handles the retrieval of all parameters to properly format them into
// sensible outputs.
//
// First, call LoadModel, this will retrieve a model from JIMM's database.
// Next, simply call GetParams.
type formatterParamsRetriever struct {
	model       *dbmodel.Model
	jujuManager *JujuManager
	api         API
}

// newFormatterParamsRetriever returns a formatterParamsRetriever.
func newFormatterParamsRetriever(j *JujuManager) *formatterParamsRetriever {
	return &formatterParamsRetriever{
		jujuManager: j,
	}
}

// GetParams retrieves the required parameters for the Juju status formatter from the currently
// loaded model. See formatterParamsRetriever.LoadModel for more information.
func (f *formatterParamsRetriever) GetParams(ctx context.Context, model dbmodel.Model) (*status.NewStatusFormatterParams, error) {
	f.model = &model

	err := f.dialModel(ctx)
	if err != nil {
		return nil, err
	}
	defer f.api.Close()

	modelStatus, err := f.getModelStatus(ctx)
	if err != nil {
		return nil, err
	}

	combinedStorage, err := f.getCombinedStorageInfo(ctx)
	if err != nil {
		return nil, err
	}

	return &status.NewStatusFormatterParams{
		ControllerName: f.model.Controller.Name,
		Status:         modelStatus,
		Storage:        combinedStorage,
		ShowRelations:  true,
		ISOTime:        true,
	}, nil
}

// dialModel dials the model currently loaded into the formatterParamsRetriever.
func (f *formatterParamsRetriever) dialModel(ctx context.Context) error {
	modelTag, ok := f.model.Tag().(names.ModelTag)
	if !ok {
		return errors.New("failed to parse model tag")
	}
	api, err := f.jujuManager.dialModelAsService(ctx, &f.model.Controller, modelTag)
	if err != nil {
		zapctx.Error(ctx, "failed to dial controller for model", zap.String("controller-uuid", f.model.Controller.UUID), zap.String("model-uuid", f.model.UUID.String), zap.Error(err))
	}
	f.api = api
	return err
}

// getModelStatus calls the FullStatus facade to return the full status for the current model
// loaded in the formatterParamsRetriever.
func (f *formatterParamsRetriever) getModelStatus(ctx context.Context) (*jujuparams.FullStatus, error) {
	modelStatus, err := f.api.Status(ctx, nil)
	if err != nil {
		zapctx.Error(ctx, "failed to call FullStatus", zap.String("controller-uuid", f.model.Controller.UUID), zap.String("model-uuid", f.model.UUID.String), zap.Error(err))
	}
	return modelStatus, err
}

func (f *formatterParamsRetriever) getCombinedStorageInfo(ctx context.Context) (*storage.CombinedStorage, error) {
	storageAPI := newStorageListAPI(ctx, f.api)

	// We use cmdCtx lightly, it's simply passed to the params but is only used for some
	// logging.
	cmdCtx, _ := jujucmd.DefaultContext()

	return storage.GetCombinedStorageInfo(storage.GetCombinedStorageInfoParams{
		Context:         cmdCtx,
		APIClient:       &storageAPI,
		Ids:             []string{},
		WantStorage:     true,
		WantVolumes:     true,
		WantFilesystems: true,
	})
}

// storageListAPI acts as a wrapper over our implementation of the juju client, seen in ./internal/jujuclient.
// This enables us to use storage.GetCombinedStorageInfo without having to c/p the logic we require.
type storageListAPI struct {
	ctx context.Context
	api API
}

// newStorageListAPI returns a new storageListAPI.
func newStorageListAPI(ctx context.Context, api API) storageListAPI {
	return storageListAPI{ctx, api}
}

// ListStorageDetails implements storage.StorageListAPI. (From Juju)
func (s *storageListAPI) ListStorageDetails(ctx context.Context) ([]jujuparams.StorageDetails, error) {
	return s.api.ListStorageDetails(s.ctx)
}

// ListFilesystems implements storage.StorageListAPI. (From Juju)
func (s *storageListAPI) ListFilesystems(ctx context.Context, machines []string) ([]jujuparams.FilesystemDetailsListResult, error) {
	return s.api.ListFilesystems(s.ctx, machines)
}

// ListVolumes implements storage.StorageListAPI. (From Juju)
func (s *storageListAPI) ListVolumes(ctx context.Context, machines []string) ([]jujuparams.VolumeDetailsListResult, error) {
	return s.api.ListVolumes(s.ctx, machines)
}

// Close implements storage.StorageListAPI. (From Juju)
func (s *storageListAPI) Close() error {
	return s.api.Close()
}
