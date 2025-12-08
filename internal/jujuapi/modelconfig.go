// Copyright 2025 Canonical.

package jujuapi

import (
	"context"

	"github.com/canonical/jimm/v3/internal/jujuapi/rpc"
	jimmversion "github.com/canonical/jimm/v3/version"
	"github.com/juju/juju/rpc/params"
)

func init() {
	facadeInit["ModelConfig"] = func(r *controllerRoot) []int {
		modelGetMethod := rpc.Method(r.ModelGet)

		r.AddMethod("ModelConfig", 3, "ModelGet", modelGetMethod)
		return []int{3}
	}
}

func (r *controllerRoot) ModelGet(ctx context.Context) (params.ModelConfigResults, error) {
	return params.ModelConfigResults{
		Config: map[string]params.ConfigValue{
			"agent-version": {
				Value:  jimmversion.VersionInfo.Version,
				Source: "jimm",
			},
		},
	}, nil
}
