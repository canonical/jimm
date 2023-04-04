package jimm

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"

	"github.com/CanonicalLtd/jimm/api/params"
	"github.com/CanonicalLtd/jimm/internal/dbmodel"
	"github.com/CanonicalLtd/jimm/internal/errors"
	"github.com/CanonicalLtd/jimm/internal/openfga"
	"github.com/itchyny/gojq"
	"go.uber.org/zap"

	"github.com/juju/juju/cmd/juju/status"
	"github.com/juju/juju/cmd/juju/storage"
	rpcparams "github.com/juju/juju/rpc/params"
	"github.com/juju/zaputil/zapctx"
)

// QueryModels queries every model available to a given user.
//
// The jqQuery must be a valid jq query and can return every result, even iterative listings.
// If a result is erroneous, for example, bad data type parsing, the resulting struct field
// Errors will contain a map from model UUID -> []error. Otherwise, the Results field
// will contain model UUID -> []Jq result.
func (j *JIMM) QueryModelsJq(ctx context.Context, user *openfga.User, jqQuery string) (params.CrossModelQueryResponse, error) {
	op := errors.Op("QueryModels")
	results := params.CrossModelQueryResponse{
		Results: make(map[string][]any),
		Errors:  make(map[string][]string),
	}

	query, err := gojq.Parse(jqQuery)
	if err != nil {
		return results, errors.E(op, err)
	}

	modelUUIDs, err := user.ListModels(ctx)
	if err != nil {
		return results, errors.E(op, err)
	}

	// We remove "model:" from the UUIDs, unfortunately that's what OpenFGA returns now after
	// recent versions.
	for i := range modelUUIDs {
		modelUUIDs[i] = strings.Split(modelUUIDs[i], ":")[1]
	}

	// Set up a formatterParamsRetriever to handle the heavy lifting
	// of each facade call and type conversion.
	retriever := newFormatterParamsRetriever(j)

	for _, id := range modelUUIDs {

		if err := retriever.LoadModel(ctx, id); err != nil {
			zapctx.Error(ctx, "failed to load model into the formatter", zap.String("model-uuid", id))
			results.Errors[id] = append(results.Errors[id], err.Error())
			continue
		}

		controllerName, modelStatus, combinedStorage, err := retriever.GetParams(ctx)
		if err != nil {
			zapctx.Error(ctx, "failed to get status formatter params", zap.String("model-uuid", id))
			results.Errors[id] = append(results.Errors[id], err.Error())
			continue
		}

		// We use very specific formatting parameters to ensure like-for-like output
		// with the default juju client installation performing a "status --format json".
		formatter := status.NewStatusFormatter(status.NewStatusFormatterParams{
			ControllerName: controllerName,
			Status:         modelStatus,
			Storage:        combinedStorage,
			ShowRelations:  true,
			ISOTime:        true,
		})

		formattedStatus, err := formatter.Format()
		if err != nil {
			zapctx.Error(ctx, "failed to format status", zap.String("model-uuid", id))
			results.Errors[id] = append(results.Errors[id], err.Error())
			continue
		}
		// We could use output.NewFormatter() from 3.0+ juju/juju, but ultimately
		// we just want some JSON output, regardless of user formatting. As such json.Marshal
		// *should* be OK. But TODO: make sure this is fine.
		fb, err := json.Marshal(formattedStatus)
		if err != nil {
			zapctx.Error(ctx, "failed to marshal formatted status", zap.String("model-uuid", id))
			results.Errors[id] = append(results.Errors[id], err.Error())
			continue
		}
		tempMap := make(map[string]any)
		if err := json.Unmarshal(fb, &tempMap); err != nil {
			return results, errors.E(op, err)
		}
		queryIter := query.RunWithContext(ctx, tempMap)

		for {
			v, ok := queryIter.Next()
			if !ok {
				break
			}

			// Jq errors can range from one failure in an iterative query to an entirely broken
			// query. As such, we simply append all to the errors field and continue to collect
			// both erreoneous and valid query results.
			if err, ok := v.(error); ok {
				results.Errors[id] = append(results.Errors[id], "jq error: "+err.Error())
				continue
			}

			results.Results[id] = append(results.Results[id], v)
		}
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
	model *dbmodel.Model
	jimm  *JIMM
	api   API
}

// newFormatterParamsRetriever returns a formatterParamsRetriever.
func newFormatterParamsRetriever(j *JIMM) *formatterParamsRetriever {
	return &formatterParamsRetriever{
		jimm: j,
	}
}

// GetParams retrieves the required parameters for the Juju status formatter from the currently
// loaded model. See formatterParamsRetriever.LoadModel for more information.
func (f *formatterParamsRetriever) GetParams(ctx context.Context) (string, *rpcparams.FullStatus, *storage.CombinedStorage, error) {
	op := errors.Op("formatterParamsRetirever.GetParams")
	if f.model == nil {
		return "", nil, nil, errors.E(op, "no model loaded")
	}

	err := f.dialModel(ctx)
	if err != nil {
		return "", nil, nil, err
	}
	defer f.api.Close()

	modelStatus, err := f.getModelStatus(ctx)
	if err != nil {
		return "", nil, nil, err
	}

	combinedStorage, err := f.getCombinedStorageInfo(ctx)
	if err != nil {
		return "", nil, nil, err
	}

	return f.model.Controller.Name, modelStatus, combinedStorage, nil
}

// LoadModel loads the model by UUID from the database into the formatterParamsRetriever.
// This MUST be called before attempting to GetParams().
func (f *formatterParamsRetriever) LoadModel(ctx context.Context, modelUUID string) error {
	model := dbmodel.Model{
		UUID: sql.NullString{String: modelUUID, Valid: true},
	}

	if err := f.jimm.Database.GetModel(ctx, &model); err != nil {
		zapctx.Error(ctx, "failed to retrieve model", zap.String("model-uuid", modelUUID))
		return err
	}
	f.model = &model
	return nil
}

// dialModel dials the model currently loaded into the formatterParamsRetriever.
func (f *formatterParamsRetriever) dialModel(ctx context.Context) error {
	api, err := f.jimm.dial(ctx, &f.model.Controller, f.model.ResourceTag())
	if err != nil {
		zapctx.Error(ctx, "failed to dial controller for model", zap.String("controller-uuid", f.model.Controller.UUID), zap.String("model-uuid", f.model.UUID.String), zap.Error(err))
	}
	f.api = api
	return err
}

// getModelStatus calls the FullStatus facade to return the full status for the current model
// loaded in the formatterParamsRetriever.
func (f *formatterParamsRetriever) getModelStatus(ctx context.Context) (*rpcparams.FullStatus, error) {
	modelStatus, err := f.api.Status(ctx, nil)
	if err != nil {
		zapctx.Error(ctx, "failed to call FullStatus", zap.String("controller-uuid", f.model.Controller.UUID), zap.String("model-uuid", f.model.UUID.String), zap.Error(err))
	}
	return modelStatus, err
}

func (f *formatterParamsRetriever) getCombinedStorageInfo(ctx context.Context) (*storage.CombinedStorage, error) {
	var err error
	combinedStorage := &storage.CombinedStorage{}

	return combinedStorage, err

}
