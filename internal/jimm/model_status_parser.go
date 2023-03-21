package jimm

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"

	"github.com/CanonicalLtd/jimm/internal/dbmodel"
	"github.com/CanonicalLtd/jimm/internal/errors"
	"github.com/CanonicalLtd/jimm/internal/openfga"
	"github.com/itchyny/gojq"
	"go.uber.org/zap"

	"github.com/juju/juju/cmd/juju/status"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/names/v4"
	"github.com/juju/zaputil/zapctx"
)

type jqResults struct {
	Results map[string][]any   `json:"results"`
	Errors  map[string][]error `json:"errors"`
}

// QueryModels queries every model available to a given user.
//
// The jqQuery must be a valid jq query and can return every result, even iterative listings.
// If a result is erroneous, for example, bad data type parsing, the resulting struct field
// Errors will contain a map from model UUID -> []error. Otherwise, the Results field
// will contain model UUID -> []Jq result.
func (j *JIMM) QueryModels(ctx context.Context, user *openfga.User, jqQuery string) (*jqResults, error) {
	op := errors.Op("todo")
	results := &jqResults{
		Results: make(map[string][]any),
		Errors:  make(map[string][]error),
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

	getModelStatus := func(modelUUID string) (*params.FullStatus, error) {
		model := dbmodel.Model{
			UUID: sql.NullString{String: modelUUID, Valid: true},
		}

		if err := j.Database.GetModel(ctx, &model); err != nil {
			zapctx.Error(ctx, "failed to retrieve model", zap.String("model-uuid", modelUUID))
			return nil, err
		}

		// We use ParseModelTag instead of NewModelTag such that the regex runs for added safety.
		tag, err := names.ParseModelTag("model-" + model.UUID.String)
		if err != nil {
			zapctx.Error(ctx, "failed to parse model tag from UUID", zap.String("model-uuid", modelUUID), zap.Error(err))
			return nil, err
		}

		api, err := j.dial(ctx, &model.Controller, tag)
		if err != nil {
			zapctx.Error(ctx, "failed to dial controller for model", zap.String("controller-uuid", model.Controller.UUID), zap.String("model-uuid", modelUUID), zap.Error(err))
			return nil, err
		}
		defer api.Close()

		modelStatus, err := api.Status(ctx, nil)
		if err != nil {
			zapctx.Error(ctx, "failed to call FullStatus", zap.String("controller-uuid", model.Controller.UUID), zap.String("model-uuid", modelUUID), zap.Error(err))
			return nil, err
		}
		return modelStatus, nil
	}

	// In return struct, have separate field of errors and results
	// errors field map of model uuid -> error.String()
	for _, id := range modelUUIDs {
		modelStatus, err := getModelStatus(id)
		if err != nil {
			// What to do with these errors?
			// TODO(ale8k): Add metrics for failures on this call
			continue
		}
		formatter := status.NewStatusFormatter(modelStatus, true)
		formattedStatus, err := formatter.Format()
		if err != nil {
			// What to do with these errors?
			// TODO(ale8k): Add metrics for failures on this call
			continue
		}
		// We could use output.NewFormatter() from 3.0+ juju/juju, but ultimately
		// we just want some JSON output, regardless of user formatting. As such json.Marshal
		// *should* be OK. But TODO: make sure this is fine.
		fb, err := json.Marshal(formattedStatus)
		if err != nil {
			// What to do with these errors?
			// TODO(ale8k): Add metrics for failures on this call
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

			if err, ok := v.(error); ok {
				results.Errors[id] = append(results.Errors[id], err)
				continue
			}

			results.Results[id] = append(results.Results[id], v)
		}
	}
	return results, nil
}
