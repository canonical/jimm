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

	// getModelStatus returns the model status for the provided modelUUID, additionally,
	// it returns the controller name that this model belongs to so that the name
	// may be passed to the formatter.
	getModelStatus := func(modelUUID string) (*rpcparams.FullStatus, string, error) {
		model := dbmodel.Model{
			UUID: sql.NullString{String: modelUUID, Valid: true},
		}

		if err := j.Database.GetModel(ctx, &model); err != nil {
			zapctx.Error(ctx, "failed to retrieve model", zap.String("model-uuid", modelUUID))
			return nil, "", err
		}

		api, err := j.dial(ctx, &model.Controller, model.ResourceTag())
		if err != nil {
			zapctx.Error(ctx, "failed to dial controller for model", zap.String("controller-uuid", model.Controller.UUID), zap.String("model-uuid", modelUUID), zap.Error(err))
			return nil, "", err
		}
		defer api.Close()

		modelStatus, err := api.Status(ctx, nil)
		if err != nil {
			zapctx.Error(ctx, "failed to call FullStatus", zap.String("controller-uuid", model.Controller.UUID), zap.String("model-uuid", modelUUID), zap.Error(err))
			return nil, "", err
		}
		return modelStatus, model.Controller.Name, nil
	}

	for _, id := range modelUUIDs {
		modelStatus, controllerName, err := getModelStatus(id)
		if err != nil {
			zapctx.Error(ctx, "failed to get model status", zap.String("model-uuid", id))
			results.Errors[id] = append(results.Errors[id], err.Error())
			continue
		}

		// We use very specific formatting parameters to ensure like-for-like output
		// with the default juju client installation performing a "status --format json".
		formatter := status.NewStatusFormatter(status.NewStatusFormatterParams{
			ControllerName: controllerName,
			Status:         modelStatus,
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
